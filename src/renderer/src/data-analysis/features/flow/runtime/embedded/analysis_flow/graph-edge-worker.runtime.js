self.EPS = 1e-6;

function clamp(val, min, max) {
  return Math.max(min, Math.min(max, val));
}

function normalizeVector(x, y) {
  const len = Math.hypot(x, y);
  if (len < self.EPS) return { x: 1, y: 0, len: 0 };
  return { x: x / len, y: y / len, len };
}

function computeEdgeEndpoints(sx, sy, tx, ty, sr, tr, lineWidth, startExtra, endExtra) {
  const dir = normalizeVector(tx - sx, ty - sy);
  if (dir.len < self.EPS) return { sx, sy, tx, ty, dir };
  const padBase = Math.max(2, lineWidth * 0.6);
  let startPad = Math.max(sr + padBase, sr + 2) + (startExtra || 0);
  let endPad = Math.max(tr + padBase, tr + 2) + (endExtra || 0);
  const maxTotal = dir.len * 0.9;
  if (maxTotal > 0 && startPad + endPad > maxTotal) {
    const scale = maxTotal / (startPad + endPad);
    startPad *= scale;
    endPad *= scale;
  }
  return {
    sx: sx + dir.x * startPad,
    sy: sy + dir.y * startPad,
    tx: tx - dir.x * endPad,
    ty: ty - dir.y * endPad,
    dir,
  };
}

function computeArrowLineTrim(arrowBack, lineWidth, sizeScale) {
  const back = Math.max(0, Number(arrowBack) || 0);
  if (back <= self.EPS) return 0;
  const lw = Math.max(0, Number(lineWidth) || 0);
  const scale = Math.max(0.1, Number(sizeScale) || 1);
  const overlap = Math.min(Math.max(0.95 * scale, lw * 0.72), back * 0.72);
  return Math.max(0, back - overlap);
}

function computeLabelSpan(absUx, absUy, w, h) {
  const spanW = absUx > self.EPS ? w / absUx : Infinity;
  const spanH = absUy > self.EPS ? h / absUy : Infinity;
  const span = Math.min(spanW, spanH);
  if (!Number.isFinite(span)) return Math.max(w, h);
  return span;
}

function estimateTextWidth(text, size) {
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
  return Math.max(1, units * Math.max(1, size));
}

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
  return [
    { sx: start.x, sy: start.y, tx: p1.x, ty: p1.y },
    { sx: p2.x, sy: p2.y, tx: end.x, ty: end.y },
  ];
}

self.onmessage = (e) => {
  const payload = e.data || {};
  const seq = payload.seq;
  const edges = payload.edges || [];
  const lite = !!payload.lite;
  const edgeStride = 12;
  const arrowStride = 9;
  const edgeFloats = [];
  const arrowFloats = [];
  const segStarts = new Int32Array(edges.length);
  const segCounts = new Int32Array(edges.length);
  const arrowStarts = new Int32Array(edges.length);
  const arrowCounts = new Int32Array(edges.length);
  let segCursor = 0;
  let arrowCursor = 0;
  edges.forEach((edge, idx) => {
    const sx = edge.sx || 0;
    const sy = edge.sy || 0;
    const tx = edge.tx || 0;
    const ty = edge.ty || 0;
    const sr = edge.sr || 18;
    const tr = edge.tr || sr;
    const lineWidth = edge.lineWidth || 2;
    const sizeScale = edge.sizeScale || 1;
    const edgeOffset = edge.edgeOffset || 0;
    const stroke = edge.stroke || [0, 0, 0, 1];
    const dash = edge.dash || { type: 0, size: 0, gap: 0 };
    const mode = String(edge.mode || "").toLowerCase();
    const arrowMode = edge.edgeArrow || (edge.showArrow === false ? "none" : "end");
    const showArrow = edge.showArrow !== false && arrowMode !== "none";
    const baseDir = normalizeVector(tx - sx, ty - sy);
    const basePerp = { x: -baseDir.y, y: baseDir.x };
    const sx0 = sx + basePerp.x * edgeOffset;
    const sy0 = sy + basePerp.y * edgeOffset;
    const tx0 = tx + basePerp.x * edgeOffset;
    const ty0 = ty + basePerp.y * edgeOffset;
    segStarts[idx] = segCursor;
    arrowStarts[idx] = arrowCursor;
    if (edge.source === edge.target) {
      const loopSteps = lite ? 14 : 22;
      const label = edge.label || "";
      const fontSize = edge.fontSize || 12;
      const labelWidth = label ? Math.max(24, estimateTextWidth(label, fontSize)) * sizeScale : 0;
      const outPad = edge.outPad || 0;
      const loopScale = edge.loopScale || 2;
      const baseR = sr + outPad + (edge.sPad || 0);
      const endR = baseR + 1 * sizeScale;
      const peak = Math.max(baseR * 0.75 * loopScale, fontSize * 1.2 * sizeScale);
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
      const arrowSize = edge.arrowSize || Math.max(6 * sizeScale, lineWidth * 3);
      const arrowTrim = computeArrowLineTrim(arrowSize, lineWidth, sizeScale);
      const trimEnd = showArrow && (arrowMode === "end" || arrowMode === "both") ? arrowTrim : 0;
      const trimStart = showArrow && (arrowMode === "start" || arrowMode === "both") ? arrowTrim : 0;
      const pStartLine = trimStart
        ? { x: pStart.x + startDir.x * trimStart, y: pStart.y + startDir.y * trimStart }
        : pStart;
      const pEndLine = trimEnd
        ? { x: pEnd.x - endDir.x * trimEnd, y: pEnd.y - endDir.y * trimEnd }
        : pEnd;
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
      for (let i = 1; i <= loopSteps; i += 1) {
        const t = i / loopSteps;
        let pt = cubicAt(pStartLine, ctrl1, ctrl2, pEndLine, t);
        if (i === loopSteps) pt = pEndLine;
        edgeFloats.push(
          prev.x,
          prev.y,
          pt.x,
          pt.y,
          lineWidth,
          stroke[0],
          stroke[1],
          stroke[2],
          stroke[3],
          dash.type,
          dash.size,
          dash.gap
        );
        segCursor += 1;
        prev = pt;
      }
      if (showArrow) {
        if (arrowMode === "end" || arrowMode === "both") {
          arrowFloats.push(pEnd.x, pEnd.y, endDir.x, endDir.y, arrowSize, stroke[0], stroke[1], stroke[2], stroke[3]);
          arrowCursor += 1;
        }
        if (arrowMode === "start" || arrowMode === "both") {
          arrowFloats.push(
            pStart.x,
            pStart.y,
            startDir.x,
            startDir.y,
            arrowSize,
            stroke[0],
            stroke[1],
            stroke[2],
            stroke[3]
          );
          arrowCursor += 1;
        }
      }
    } else {
      const isDouble = mode === "double";
      const outPad = edge.outPad || 0;
      const holePad = edge.holePad || 0;
      const extraS = outPad + holePad + (edge.sPad || 0);
      const extraT = outPad + holePad + (edge.tPad || 0);
      const endpoints = computeEdgeEndpoints(sx0, sy0, tx0, ty0, sr, tr, lineWidth, extraS, extraT);
      const sxTip = endpoints.sx;
      const syTip = endpoints.sy;
      const txTip = endpoints.tx;
      const tyTip = endpoints.ty;
      const dir = normalizeVector(txTip - sxTip, tyTip - syTip);
      const arrowSize = edge.arrowSize || Math.max(6 * sizeScale, lineWidth * 3);
      const arrowTrim = computeArrowLineTrim(arrowSize, lineWidth, sizeScale);
      const perp = { x: -dir.y, y: dir.x };
      const absUx = Math.abs(dir.x);
      const absUy = Math.abs(dir.y);
      const labelOffsetY = Number(edge.labelOffsetY) || 0;
      const mid = { x: (sx0 + tx0) / 2, y: (sy0 + ty0) / 2 + labelOffsetY };
      if (isDouble) {
        const half = (edge.gap || 0) / 2;
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
        const endTrimTop = showArrow && showEnd ? arrowTrim : 0;
        const endTrimBot = showArrow && showStart ? arrowTrim : 0;
        const topStart = { x: sOutTop.x, y: sOutTop.y };
        const topEnd = { x: tOutTop.x - dir.x * endTrimTop, y: tOutTop.y - dir.y * endTrimTop };
        const botStart = { x: tOutBot.x, y: tOutBot.y };
        const botEnd = { x: sOutBot.x + dir.x * endTrimBot, y: sOutBot.y + dir.y * endTrimBot };
        const labelTop = edge.labelTop || "";
        const labelBottom = edge.labelBottom || "";
        const labelSize = Math.max(8, (edge.fontSize || 12) - 1);
        const labelPad = Math.max(3, Math.min(10, Math.round(labelSize * 0.42))) * sizeScale;
        const topDim = labelTop
          ? {
              w: Math.max(24, estimateTextWidth(labelTop, labelSize)) * sizeScale,
              h: Math.max(12, labelSize + 6) * sizeScale,
            }
          : { w: 0, h: 0 };
        const botDim = labelBottom
          ? {
              w: Math.max(24, estimateTextWidth(labelBottom, labelSize)) * sizeScale,
              h: Math.max(12, labelSize + 6) * sizeScale,
            }
          : { w: 0, h: 0 };
        const topMid = { x: (sTop.x + tTop.x) / 2, y: (sTop.y + tTop.y) / 2 + labelOffsetY };
        const botMid = { x: (sBot.x + tBot.x) / 2, y: (sBot.y + tBot.y) / 2 + labelOffsetY };
        const topGap = labelTop ? computeLabelSpan(absUx, absUy, topDim.w, topDim.h) + labelPad * 2 : 0;
        const botGap = labelBottom ? computeLabelSpan(absUx, absUy, botDim.w, botDim.h) + labelPad * 2 : 0;
        buildGapSegments(topStart, topEnd, topGap, topMid).forEach((seg) => {
          edgeFloats.push(seg.sx, seg.sy, seg.tx, seg.ty, lineWidth, stroke[0], stroke[1], stroke[2], stroke[3], dash.type, dash.size, dash.gap);
          segCursor += 1;
        });
        buildGapSegments(botStart, botEnd, botGap, botMid).forEach((seg) => {
          edgeFloats.push(seg.sx, seg.sy, seg.tx, seg.ty, lineWidth, stroke[0], stroke[1], stroke[2], stroke[3], dash.type, dash.size, dash.gap);
          segCursor += 1;
        });
      } else {
        const showEnd = arrowMode === "end" || arrowMode === "both";
        const showStart = arrowMode === "start" || arrowMode === "both";
        const startTrim = showArrow && showStart ? arrowTrim : 0;
        const endTrim = showArrow && showEnd ? arrowTrim : 0;
        const sLine = { x: sxTip + dir.x * startTrim, y: syTip + dir.y * startTrim };
        const tLine = { x: txTip - dir.x * endTrim, y: tyTip - dir.y * endTrim };
        const labelSize = Math.max(8, (edge.fontSize || 12) - 1);
        const labelPad = Math.max(3, Math.min(10, Math.round(labelSize * 0.42))) * sizeScale;
        const label = edge.label || "";
        const labelDim = label
          ? {
              w: Math.max(24, estimateTextWidth(label, labelSize)) * sizeScale,
              h: Math.max(12, labelSize + 6) * sizeScale,
            }
          : { w: 0, h: 0 };
        const gapLen = label ? computeLabelSpan(absUx, absUy, labelDim.w, labelDim.h) + labelPad * 2 : 0;
        buildGapSegments(sLine, tLine, gapLen, mid).forEach((seg) => {
          edgeFloats.push(seg.sx, seg.sy, seg.tx, seg.ty, lineWidth, stroke[0], stroke[1], stroke[2], stroke[3], dash.type, dash.size, dash.gap);
          segCursor += 1;
        });
      }
      if (showArrow) {
        if (isDouble) {
          const half = (edge.gap || 0) / 2;
          if (arrowMode === "end" || arrowMode === "both") {
            arrowFloats.push(txTip + perp.x * half, tyTip + perp.y * half, dir.x, dir.y, arrowSize, stroke[0], stroke[1], stroke[2], stroke[3]);
            arrowCursor += 1;
          }
          if (arrowMode === "start" || arrowMode === "both") {
            arrowFloats.push(sxTip - perp.x * half, syTip - perp.y * half, -dir.x, -dir.y, arrowSize, stroke[0], stroke[1], stroke[2], stroke[3]);
            arrowCursor += 1;
          }
        } else {
          if (arrowMode === "end" || arrowMode === "both") {
            arrowFloats.push(txTip, tyTip, dir.x, dir.y, arrowSize, stroke[0], stroke[1], stroke[2], stroke[3]);
            arrowCursor += 1;
          }
          if (arrowMode === "start" || arrowMode === "both") {
            arrowFloats.push(sxTip, syTip, -dir.x, -dir.y, arrowSize, stroke[0], stroke[1], stroke[2], stroke[3]);
            arrowCursor += 1;
          }
        }
      }
    }
    segCounts[idx] = segCursor - segStarts[idx];
    arrowCounts[idx] = arrowCursor - arrowStarts[idx];
  });
  const edgeData = new Float32Array(edgeFloats);
  const arrowData = new Float32Array(arrowFloats);
  self.postMessage(
    { seq, edgeData, arrowData, segStarts, segCounts, arrowStarts, arrowCounts },
    [edgeData.buffer, arrowData.buffer, segStarts.buffer, segCounts.buffer, arrowStarts.buffer, arrowCounts.buffer]
  );
};
