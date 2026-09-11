/* global window */
(() => {
  const EPS = 1e-6;
  const graphEnginePrimitives = window.__ANALYTIX_GRAPH_ENGINE_PRIMITIVES__ || null;
  if (!graphEnginePrimitives) {
    throw new Error("graph engine primitives missing for edge routing");
  }
  const {
    clamp,
    normalizeVector,
    computeEdgeEndpoints,
  } = graphEnginePrimitives;

  function buildGapSegments(start, end, gapLen, center) {
    const dx = end.x - start.x;
    const dy = end.y - start.y;
    const len = Math.hypot(dx, dy);
    if (!gapLen || gapLen <= 0 || len < 2) {
      return [{ sx: start.x, sy: start.y, tx: end.x, ty: end.y }];
    }
    const maxGap = Math.max(0, len - 8);
    const safeGap = clamp(gapLen, 0, maxGap);
    if (safeGap <= 0 || safeGap >= len) {
      return [{ sx: start.x, sy: start.y, tx: end.x, ty: end.y }];
    }
    const ux = dx / len;
    const uy = dy / len;
    let t = 0.5;
    if (center) {
      const cx = center.x - start.x;
      const cy = center.y - start.y;
      t = clamp((cx * ux + cy * uy) / len, 0, 1);
    }
    const half = safeGap / 2;
    let gapStart = t * len - half;
    let gapEnd = t * len + half;
    if (gapStart < 0) {
      gapEnd = clamp(gapEnd - gapStart, 0, len);
      gapStart = 0;
    }
    if (gapEnd > len) {
      gapStart = clamp(gapStart - (gapEnd - len), 0, len);
      gapEnd = len;
    }
    const p1 = { x: start.x + ux * gapStart, y: start.y + uy * gapStart };
    const p2 = { x: start.x + ux * gapEnd, y: start.y + uy * gapEnd };
    const segs = [];
    if (Math.hypot(p1.x - start.x, p1.y - start.y) > EPS) {
      segs.push({ sx: start.x, sy: start.y, tx: p1.x, ty: p1.y });
    }
    if (Math.hypot(end.x - p2.x, end.y - p2.y) > EPS) {
      segs.push({ sx: p2.x, sy: p2.y, tx: end.x, ty: end.y });
    }
    if (!segs.length) segs.push({ sx: start.x, sy: start.y, tx: end.x, ty: end.y });
    return segs;
  }

  function buildOrthogonalSegments(start, end, axis = "horizontal", bias = 0.5, sharedCoord = null) {
    const dx = end.x - start.x;
    const dy = end.y - start.y;
    if (Math.abs(dx) < EPS || Math.abs(dy) < EPS) {
      return [{ sx: start.x, sy: start.y, tx: end.x, ty: end.y }];
    }
    const k = clamp(Number.isFinite(bias) ? bias : 0.5, 0.18, 0.82);
    const segs = [];
    if (axis === "vertical") {
      const myRaw = sharedCoord == null ? NaN : Number(sharedCoord);
      const my = Number.isFinite(myRaw)
        ? clamp(myRaw, Math.min(start.y, end.y), Math.max(start.y, end.y))
        : start.y + dy * k;
      segs.push({ sx: start.x, sy: start.y, tx: start.x, ty: my });
      segs.push({ sx: start.x, sy: my, tx: end.x, ty: my });
      segs.push({ sx: end.x, sy: my, tx: end.x, ty: end.y });
    } else {
      const mxRaw = sharedCoord == null ? NaN : Number(sharedCoord);
      const mx = Number.isFinite(mxRaw)
        ? clamp(mxRaw, Math.min(start.x, end.x), Math.max(start.x, end.x))
        : start.x + dx * k;
      segs.push({ sx: start.x, sy: start.y, tx: mx, ty: start.y });
      segs.push({ sx: mx, sy: start.y, tx: mx, ty: end.y });
      segs.push({ sx: mx, sy: end.y, tx: end.x, ty: end.y });
    }
    const out = segs.filter((seg) => Math.hypot(seg.tx - seg.sx, seg.ty - seg.sy) > EPS);
    if (!out.length) out.push({ sx: start.x, sy: start.y, tx: end.x, ty: end.y });
    return out;
  }

  function trimPolylineSegments(segments, startTrim, endTrim) {
    const out = (segments || []).map((seg) => ({ ...seg }));
    let trimHead = Math.max(0, Number(startTrim) || 0);
    for (let i = 0; i < out.length && trimHead > EPS; i += 1) {
      const seg = out[i];
      const dx = seg.tx - seg.sx;
      const dy = seg.ty - seg.sy;
      const len = Math.hypot(dx, dy);
      if (len <= EPS) {
        out.splice(i, 1);
        i -= 1;
        continue;
      }
      if (len <= trimHead + EPS) {
        trimHead -= len;
        out.splice(i, 1);
        i -= 1;
        continue;
      }
      const ux = dx / len;
      const uy = dy / len;
      seg.sx += ux * trimHead;
      seg.sy += uy * trimHead;
      trimHead = 0;
    }
    let trimTail = Math.max(0, Number(endTrim) || 0);
    for (let i = out.length - 1; i >= 0 && trimTail > EPS; i -= 1) {
      const seg = out[i];
      const dx = seg.tx - seg.sx;
      const dy = seg.ty - seg.sy;
      const len = Math.hypot(dx, dy);
      if (len <= EPS) {
        out.splice(i, 1);
        continue;
      }
      if (len <= trimTail + EPS) {
        trimTail -= len;
        out.splice(i, 1);
        continue;
      }
      const ux = dx / len;
      const uy = dy / len;
      seg.tx -= ux * trimTail;
      seg.ty -= uy * trimTail;
      trimTail = 0;
    }
    return out;
  }

  function segmentLength(seg) {
    if (!seg) return 0;
    const dx = (Number(seg.tx) || 0) - (Number(seg.sx) || 0);
    const dy = (Number(seg.ty) || 0) - (Number(seg.sy) || 0);
    return Math.hypot(dx, dy);
  }

  function segmentAxis(seg) {
    if (!seg) return "";
    const dx = Math.abs((Number(seg.tx) || 0) - (Number(seg.sx) || 0));
    const dy = Math.abs((Number(seg.ty) || 0) - (Number(seg.sy) || 0));
    if (dx <= EPS && dy <= EPS) return "";
    return dx >= dy ? "horizontal" : "vertical";
  }

  function pickSegmentIndex(segments, preferAxis = "") {
    const list = Array.isArray(segments) ? segments : [];
    if (!list.length) return -1;
    const pref = preferAxis === "vertical" ? "vertical" : preferAxis === "horizontal" ? "horizontal" : "";
    let best = -1;
    let bestLen = -1;
    list.forEach((seg, idx) => {
      const len = segmentLength(seg);
      if (len <= EPS) return;
      if (pref && segmentAxis(seg) !== pref) return;
      if (len > bestLen) {
        bestLen = len;
        best = idx;
      }
    });
    if (best >= 0) return best;
    list.forEach((seg, idx) => {
      const len = segmentLength(seg);
      if (len <= EPS) return;
      if (len > bestLen) {
        bestLen = len;
        best = idx;
      }
    });
    return best >= 0 ? best : 0;
  }

  function collectAxisSegmentIndices(segments, axis) {
    const list = Array.isArray(segments) ? segments : [];
    const pref = axis === "vertical" ? "vertical" : "horizontal";
    const out = [];
    list.forEach((seg, idx) => {
      if (segmentLength(seg) <= EPS) return;
      if (segmentAxis(seg) !== pref) return;
      out.push(idx);
    });
    return out;
  }

  function pointDistSq(a, b) {
    if (!a || !b) return Infinity;
    const dx = (Number(a.x) || 0) - (Number(b.x) || 0);
    const dy = (Number(a.y) || 0) - (Number(b.y) || 0);
    return dx * dx + dy * dy;
  }

  function resolveEdgeArrowMode(model) {
    const raw = model?.edgeArrow || (model?.showArrow === false ? "none" : "end");
    if (raw === "start" || raw === "end" || raw === "both" || raw === "none") return raw;
    return "none";
  }

  function resolveFlowOriginSide(model) {
    const arrowMode = resolveEdgeArrowMode(model);
    if (arrowMode === "end") return "source";
    if (arrowMode === "start") return "target";
    return null;
  }

  function buildPlainEdgeLabelSidePayload(edge) {
    const source = edge?.getSource?.();
    const target = edge?.getTarget?.();
    const sm = source?.getModel?.() || null;
    const tm = target?.getModel?.() || null;
    return {
      source: sm
        ? {
            x: Number(sm.x) || 0,
            y: Number(sm.y) || 0,
            degree: Number(source?.getEdges?.()?.length) || 0,
          }
        : null,
      target: tm
        ? {
            x: Number(tm.x) || 0,
            y: Number(tm.y) || 0,
            degree: Number(target?.getEdges?.()?.length) || 0,
          }
        : null,
      model: edge?.getModel?.() || {},
    };
  }

  function resolvePlainLabelSide(payload = {}, axisHint = "horizontal") {
    const source = payload?.source && typeof payload.source === "object" ? payload.source : null;
    const target = payload?.target && typeof payload.target === "object" ? payload.target : null;
    if (!source || !target) return "source";

    const sDeg = Number(source?.degree) || 0;
    const tDeg = Number(target?.degree) || 0;
    if (sDeg !== tDeg) return sDeg < tDeg ? "source" : "target";

    const m = payload?.model && typeof payload.model === "object" ? payload.model : {};
    const preferDegreeSide = !!m?.edgeLabelPreferDegree || !!m?.edgeLabelIgnoreFlowOrigin;
    if (!preferDegreeSide) {
      const flowSide = resolveFlowOriginSide(m);
      if (flowSide) return flowSide;
    }

    const sx = Number(source.x) || 0;
    const sy = Number(source.y) || 0;
    const tx = Number(target.x) || 0;
    const ty = Number(target.y) || 0;
    const axis = axisHint === "vertical" ? "vertical" : "horizontal";
    if (axis === "vertical") {
      if (Math.abs(sy - ty) > EPS) return sy <= ty ? "source" : "target";
      if (Math.abs(sx - tx) > EPS) return sx <= tx ? "source" : "target";
      return "source";
    }
    if (Math.abs(sx - tx) > EPS) return sx <= tx ? "source" : "target";
    if (Math.abs(sy - ty) > EPS) return sy <= ty ? "source" : "target";
    return "source";
  }

  function resolveLabelSide(edge, axisHint = "horizontal") {
    return resolvePlainLabelSide(buildPlainEdgeLabelSidePayload(edge), axisHint);
  }

  function resolvePlainOrthogonalLabelSegmentIndex(payload = {}, routeData, axis = "horizontal") {
    const segments = routeData?.segments || [];
    if (!segments.length) return -1;
    const prefAxis = axis === "vertical" ? "vertical" : "horizontal";
    const axisIdx = collectAxisSegmentIndices(segments, prefAxis);
    if (!axisIdx.length) return pickSegmentIndex(segments, prefAxis);
    if (axisIdx.length === 1) return axisIdx[0];

    const firstIdx = axisIdx[0];
    const lastIdx = axisIdx[axisIdx.length - 1];
    const side = resolvePlainLabelSide(payload, prefAxis);
    const sideIdx = side === "target" ? lastIdx : firstIdx;
    if (segmentLength(segments[sideIdx]) > EPS) return sideIdx;

    const firstLen = segmentLength(segments[firstIdx]);
    const lastLen = segmentLength(segments[lastIdx]);
    if (Math.abs(firstLen - lastLen) > 0.5) {
      return firstLen >= lastLen ? firstIdx : lastIdx;
    }
    return firstIdx;
  }

  function resolveOrthogonalLabelSegmentIndex(edge, routeData, axis = "horizontal") {
    return resolvePlainOrthogonalLabelSegmentIndex(buildPlainEdgeLabelSidePayload(edge), routeData, axis);
  }

  function resolvePlainOrthogonalLabelPoint(payload = {}, routeData, axis = "horizontal", opts = {}) {
    const idx = resolvePlainOrthogonalLabelSegmentIndex(payload, routeData, axis);
    if (!Number.isInteger(idx) || idx < 0) return { point: routeData?.labelPoint || null, index: idx };
    const seg = routeData?.segments?.[idx];
    if (!seg) return { point: routeData?.labelPoint || null, index: idx };
    const centerOnSegment = opts?.centerOnSegment === true;
    if (centerOnSegment) {
      return {
        point: {
          x: ((Number(seg.sx) || 0) + (Number(seg.tx) || 0)) / 2,
          y: ((Number(seg.sy) || 0) + (Number(seg.ty) || 0)) / 2,
        },
        index: idx,
      };
    }
    const source = payload?.source && typeof payload.source === "object" ? payload.source : null;
    const target = payload?.target && typeof payload.target === "object" ? payload.target : null;
    const side = resolvePlainLabelSide(payload, axis);
    const ref =
      side === "source"
        ? { x: Number(source?.x) || 0, y: Number(source?.y) || 0 }
        : { x: Number(target?.x) || 0, y: Number(target?.y) || 0 };
    const a = { x: Number(seg.sx) || 0, y: Number(seg.sy) || 0 };
    const b = { x: Number(seg.tx) || 0, y: Number(seg.ty) || 0 };
    let near = a;
    let far = b;
    if (pointDistSq(b, ref) < pointDistSq(a, ref)) {
      near = b;
      far = a;
    }
    const dir = normalizeVector(far.x - near.x, far.y - near.y);
    const len = Math.hypot(far.x - near.x, far.y - near.y);
    const baseOffset = Math.max(24, Number(opts?.sideOffset) || 92);
    const midRatioRaw = Number(opts?.middleRatio);
    const middleRatio = Number.isFinite(midRatioRaw) ? clamp(midRatioRaw, 0.18, 0.5) : NaN;
    const capRatioRaw = Number(opts?.sideCapRatio);
    const sideCapRatio = Number.isFinite(capRatioRaw) ? clamp(capRatioRaw, 0.38, 0.58) : 0.52;
    const endInset = Math.max(8, Number(opts?.endInset) || 12);
    const maxUsable = Math.max(0, len - endInset);
    const cappedBase = Math.min(baseOffset, len * sideCapRatio);
    let desired = baseOffset;
    if (Number.isFinite(middleRatio)) {
      desired = Math.max(cappedBase, len * middleRatio);
    } else {
      desired = cappedBase;
    }
    const d = Math.min(desired, maxUsable);
    return { point: { x: near.x + dir.x * d, y: near.y + dir.y * d }, index: idx };
  }

  function resolveOrthogonalLabelPoint(edge, routeData, axis = "horizontal", opts = {}) {
    return resolvePlainOrthogonalLabelPoint(
      buildPlainEdgeLabelSidePayload(edge),
      routeData,
      axis,
      opts
    );
  }

  function computeOrthogonalEndpoints(
    sx,
    sy,
    tx,
    ty,
    sr,
    tr,
    lineWidth,
    startExtra = 0,
    endExtra = 0,
    axis = "horizontal"
  ) {
    const orthAxis = axis === "vertical" ? "vertical" : "horizontal";
    const delta = orthAxis === "vertical" ? ty - sy : tx - sx;
    if (Math.abs(delta) < EPS) {
      return computeEdgeEndpoints(sx, sy, tx, ty, sr, tr, lineWidth, startExtra, endExtra);
    }
    const sign = delta >= 0 ? 1 : -1;
    const padBase = Math.max(2, lineWidth * 0.6);
    let startPad = Math.max(sr + padBase, sr + 2) + (Number(startExtra) || 0);
    let endPad = Math.max(tr + padBase, tr + 2) + (Number(endExtra) || 0);
    const span = Math.abs(delta);
    const maxTotal = span * 0.9;
    if (maxTotal > 0 && startPad + endPad > maxTotal) {
      const scale = maxTotal / (startPad + endPad);
      startPad *= scale;
      endPad *= scale;
    }
    if (orthAxis === "vertical") {
      return {
        sx,
        sy: sy + sign * startPad,
        tx,
        ty: ty - sign * endPad,
        dir: { x: 0, y: sign, len: span },
      };
    }
    return {
      sx: sx + sign * startPad,
      sy,
      tx: tx - sign * endPad,
      ty,
      dir: { x: sign, y: 0, len: span },
    };
  }

  function buildOrthogonalRouteData(
    start,
    end,
    axis = "horizontal",
    bias = 0.5,
    startTrim = 0,
    endTrim = 0,
    preferAxis = axis,
    sharedCoord = null
  ) {
    const baseSegs = buildOrthogonalSegments(start, end, axis, bias, sharedCoord);
    const trimmed = trimPolylineSegments(baseSegs, startTrim, endTrim);
    const segs = trimmed.length ? trimmed : baseSegs;
    const first = segs[0] || baseSegs[0] || { sx: start.x, sy: start.y, tx: end.x, ty: end.y };
    const last = segs[segs.length - 1] || baseSegs[baseSegs.length - 1] || first;
    const labelIndex = pickSegmentIndex(segs, preferAxis);
    const labelSeg = segs[labelIndex];
    const labelPoint = labelSeg
      ? { x: (labelSeg.sx + labelSeg.tx) / 2, y: (labelSeg.sy + labelSeg.ty) / 2 }
      : { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 };
    return {
      baseSegs,
      segments: segs,
      startDir: normalizeVector((first?.tx || start.x) - (first?.sx || start.x), (first?.ty || start.y) - (first?.sy || start.y)),
      endDir: normalizeVector((last?.tx || end.x) - (last?.sx || end.x), (last?.ty || end.y) - (last?.sy || end.y)),
      labelSegmentIndex: labelIndex,
      labelPoint,
    };
  }

  function applyGapToSegments(segments, gapLen, center, opts = {}) {
    const list = segments || [];
    if (!gapLen || gapLen <= 0 || !list.length) return list;
    const preferredAxis =
      String(opts?.preferAxis || "").toLowerCase() === "vertical"
        ? "vertical"
        : String(opts?.preferAxis || "").toLowerCase() === "horizontal"
        ? "horizontal"
        : "";
    const picked = Number(opts?.pickIndex);
    let pick = -1;
    if (Number.isInteger(picked) && picked >= 0 && picked < list.length) {
      if (segmentLength(list[picked]) > EPS) pick = picked;
    }
    if (pick < 0) pick = pickSegmentIndex(list, preferredAxis);
    if (pick < 0) pick = pickSegmentIndex(list, "");
    if (pick < 0) return list;
    const maxLen = segmentLength(list[pick]);
    if (maxLen <= EPS) return list;
    const target = list[pick];
    const gapped = buildGapSegments(
      { x: target.sx, y: target.sy },
      { x: target.tx, y: target.ty },
      gapLen,
      center
    );
    const out = [];
    list.forEach((seg, idx) => {
      if (idx === pick) {
        gapped.forEach((g) => out.push(g));
      } else {
        out.push(seg);
      }
    });
    return out;
  }

  function sealPolylineJoints(segments, lineWidth, opts = {}) {
    const list = Array.isArray(segments) ? segments.map((seg) => ({ ...seg })) : [];
    if (list.length < 2) return list;
    const lw = Math.max(0, Number(lineWidth) || 0);
    const overlapRaw = Number(opts?.overlap);
    const overlap = Number.isFinite(overlapRaw) && overlapRaw > 0 ? overlapRaw : Math.max(0.8, lw * 0.52);
    if (overlap <= EPS) return list;
    const tolRaw = Number(opts?.tolerance);
    const tol = Number.isFinite(tolRaw) && tolRaw > 0 ? tolRaw : Math.max(EPS * 4, 0.25);
    for (let i = 0; i < list.length - 1; i += 1) {
      const a = list[i];
      const b = list[i + 1];
      if (!a || !b) continue;
      const ax = Number(a.tx) || 0;
      const ay = Number(a.ty) || 0;
      const bx = Number(b.sx) || 0;
      const by = Number(b.sy) || 0;
      if (Math.hypot(bx - ax, by - ay) > tol) continue;
      const asx = Number(a.sx) || 0;
      const asy = Number(a.sy) || 0;
      const btx = Number(b.tx) || 0;
      const bty = Number(b.ty) || 0;
      const dirA = normalizeVector(ax - asx, ay - asy);
      const dirB = normalizeVector(btx - bx, bty - by);
      if (dirA.len <= EPS || dirB.len <= EPS) continue;
      const turn = Math.abs(dirA.x * dirB.y - dirA.y * dirB.x);
      if (turn < 0.05) continue;
      const extA = Math.min(overlap, dirA.len * 0.45);
      const extB = Math.min(overlap, dirB.len * 0.45);
      if (extA > EPS) {
        a.tx = ax + dirA.x * extA;
        a.ty = ay + dirA.y * extA;
      }
      if (extB > EPS) {
        b.sx = bx - dirB.x * extB;
        b.sy = by - dirB.y * extB;
      }
    }
    return list;
  }

  function resolveOrthogonalLabelAnchorOpts(model, axis = "horizontal", sizeScale = 1) {
    const orient = axis === "vertical" ? "vertical" : "horizontal";
    const sideOffsetRaw = Number(model?.edgeLabelSideOffset);
    const middleRatioRaw = Number(model?.edgeLabelMiddleRatio);
    const sideCapRatioRaw = Number(model?.edgeLabelSideCapRatio);
    const defaultOffset = 96;
    const defaultMiddle = orient === "horizontal" ? 0.47 : 0.38;
    const defaultCap = orient === "horizontal" ? 0.52 : 0.56;
    return {
      sideOffset: Math.max(16, Number.isFinite(sideOffsetRaw) ? sideOffsetRaw : defaultOffset) * (Number(sizeScale) || 1),
      middleRatio: Number.isFinite(middleRatioRaw) ? middleRatioRaw : defaultMiddle,
      sideCapRatio: Number.isFinite(sideCapRatioRaw) ? sideCapRatioRaw : defaultCap,
    };
  }

  function resolveEdgeLabelYOffset(model, sizeScale = 1) {
    const raw = Number(model?.edgeLabelOffsetY ?? model?.labelOffsetY);
    if (!Number.isFinite(raw)) return 0;
    return raw * (Number(sizeScale) || 1);
  }

  window.__ANALYTIX_GRAPH_EDGE_ROUTING__ = {
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
    buildPlainEdgeLabelSidePayload,
    resolvePlainLabelSide,
    resolveLabelSide,
    resolvePlainOrthogonalLabelSegmentIndex,
    resolveOrthogonalLabelSegmentIndex,
    resolvePlainOrthogonalLabelPoint,
    resolveOrthogonalLabelPoint,
    computeOrthogonalEndpoints,
    buildOrthogonalRouteData,
    applyGapToSegments,
    sealPolylineJoints,
    resolveOrthogonalLabelAnchorOpts,
    resolveEdgeLabelYOffset,
  };
})();
