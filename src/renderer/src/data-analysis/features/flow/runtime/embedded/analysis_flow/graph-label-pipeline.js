/* global window */
(() => {
  const EPS = 1e-6;
  const NODE_LABEL_MAIN_MAX_WIDTH_PX = 220;
  const NODE_LABEL_SUB_MAX_WIDTH_PX = 260;
  const NODE_LABEL_SINGLE_MAX_WIDTH_PX = 248;
  const NODE_LABEL_AVOID_PAD_PX = 3;
  const primitives = window.__ANALYTIX_GRAPH_ENGINE_PRIMITIVES__ || null;
  if (!primitives) {
    throw new Error("graph engine primitives missing for label pipeline");
  }
  const { clamp, asBool, truncateTextByWorldWidth, estimateTextBox } = primitives;
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__;
  if (
    !piiProjection ||
    typeof piiProjection.projectDetected !== "function" ||
    typeof piiProjection.projectField !== "function"
  ) {
    throw new Error("ordinary PII projection missing for label pipeline");
  }

  function resolveNodeLabelRows(model, sizeScale = 1) {
    const rows = [];
    const hideNodeLabel = asBool(model?.nodeLabelHidden, false);
    const baseSize = clamp(Number(model?.fontSize ?? 13), 8, 36);
    const subSize = Math.max(8, baseSize - 1);
    if (hideNodeLabel) return { rows, baseSize, subSize };
    const name = piiProjection.projectDetected(model?.name || "");
    const title = piiProjection.projectField("node_id", model?.title || model?.label || model?.id || "");
    const displayId = piiProjection.projectField("node_id", model?.displayId || model?.display_id || "");
    const scale = Number(sizeScale) || 1;
    const rWorld = (Number(model?.r) || 18) * scale;
    const offsetBase = rWorld + 16 * scale;
    const offsetSub = rWorld + 34 * scale;
    const offsetSingle = rWorld + 20 * scale;
    const maxMainPx = Number(model?.nodeLabelMainMaxWidth);
    const maxSubPx = Number(model?.nodeLabelSubMaxWidth);
    const maxSinglePx = Number(model?.nodeLabelSingleMaxWidth);
    const mainMaxWorld = (Number.isFinite(maxMainPx) && maxMainPx > 0 ? maxMainPx : NODE_LABEL_MAIN_MAX_WIDTH_PX) * scale;
    const subMaxWorld = (Number.isFinite(maxSubPx) && maxSubPx > 0 ? maxSubPx : NODE_LABEL_SUB_MAX_WIDTH_PX) * scale;
    const singleMaxWorld =
      (Number.isFinite(maxSinglePx) && maxSinglePx > 0 ? maxSinglePx : NODE_LABEL_SINGLE_MAX_WIDTH_PX) * scale;
    if (name) {
      const mainText = truncateTextByWorldWidth(name || title, baseSize, mainMaxWorld, scale);
      const subText = truncateTextByWorldWidth(displayId || title, subSize, subMaxWorld, scale);
      if (mainText) rows.push({ text: mainText, size: baseSize, offsetY: offsetBase, tier: "main" });
      if (subText) rows.push({ text: subText, size: subSize, offsetY: offsetSub, tier: "sub" });
    } else {
      const singleText = truncateTextByWorldWidth(displayId || title, baseSize, singleMaxWorld, scale);
      if (singleText) rows.push({ text: singleText, size: baseSize, offsetY: offsetSingle, tier: "single" });
    }
    return { rows, baseSize, subSize };
  }

  function resolveEdgeLabelRows(model) {
    return {
      label: piiProjection.projectField(model?.userCreated ? "node_id" : "", model?.label || ""),
      labelTop: piiProjection.projectDetected(model?.labelTop || ""),
      labelBottom: piiProjection.projectDetected(model?.labelBottom || ""),
    };
  }

  function buildNodeLabelAvoidRects(model, sizeScale = 1) {
    if (!model || typeof model !== "object") return [];
    const nx = Number(model.x);
    const ny = Number(model.y);
    if (!Number.isFinite(nx) || !Number.isFinite(ny)) return [];
    const layout = resolveNodeLabelRows(model, sizeScale);
    const pad = Math.max(1, NODE_LABEL_AVOID_PAD_PX * (Number(sizeScale) || 1));
    const rects = [];
    layout.rows.forEach((row) => {
      const text = String(row?.text || "");
      if (!text) return;
      const dim = estimateTextBox(text, row.size, sizeScale);
      if (dim.w <= EPS || dim.h <= EPS) return;
      const cy = ny + (Number(row.offsetY) || 0);
      rects.push({
        minX: nx - dim.w / 2 - pad,
        maxX: nx + dim.w / 2 + pad,
        minY: cy - dim.h / 2 - pad,
        maxY: cy + dim.h / 2 + pad,
      });
    });
    return rects;
  }

  window.__ANALYTIX_GRAPH_LABEL_PIPELINE__ = {
    NODE_LABEL_MAIN_MAX_WIDTH_PX,
    NODE_LABEL_SUB_MAX_WIDTH_PX,
    NODE_LABEL_SINGLE_MAX_WIDTH_PX,
    NODE_LABEL_AVOID_PAD_PX,
    resolveNodeLabelRows,
    resolveEdgeLabelRows,
    buildNodeLabelAvoidRects,
  };
})();
