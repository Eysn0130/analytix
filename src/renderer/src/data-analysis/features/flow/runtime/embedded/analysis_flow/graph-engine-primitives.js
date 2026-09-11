(() => {
  const EPS = 1e-6;
  const WEBGL_CLEAR_FALLBACK = [246 / 255, 247 / 255, 251 / 255, 1];
  const TEXT_ELLIPSIS = "...";

  function clamp(val, min, max) {
    return Math.max(min, Math.min(max, val));
  }

  function normalizeTextQualityMode(value) {
    const raw = String(value || "").trim().toLowerCase();
    if (raw === "gpu" || raw === "text-atlas" || raw === "textatlas" || raw === "gpu-text-atlas") {
      return "gpu";
    }
    return "gpu";
  }

  function asBool(value, fallback = false) {
    if (value == null) return !!fallback;
    if (typeof value === "boolean") return value;
    if (typeof value === "number") return Number.isFinite(value) && value !== 0;
    if (typeof value === "string") {
      const s = value.trim().toLowerCase();
      if (!s) return false;
      if (["0", "false", "no", "off", "null", "undefined", "nan"].includes(s)) return false;
      if (["1", "true", "yes", "on"].includes(s)) return true;
      return true;
    }
    return !!value;
  }

  function parseColor(input, fallback = [0, 0, 0, 1]) {
    if (!input) return fallback.slice();
    const str = String(input).trim();
    if (!str) return fallback.slice();
    if (str === "transparent") return [0, 0, 0, 0];
    if (str.startsWith("#")) {
      let hex = str.slice(1);
      if (hex.length === 3) {
        hex = hex
          .split("")
          .map((c) => c + c)
          .join("");
      }
      if (hex.length === 6 || hex.length === 8) {
        const r = parseInt(hex.slice(0, 2), 16);
        const g = parseInt(hex.slice(2, 4), 16);
        const b = parseInt(hex.slice(4, 6), 16);
        const a = hex.length === 8 ? parseInt(hex.slice(6, 8), 16) / 255 : 1;
        return [r / 255, g / 255, b / 255, a];
      }
      return fallback.slice();
    }
    const rgbMatch = str.match(/rgba?\(([^)]+)\)/i);
    if (rgbMatch) {
      const parts = rgbMatch[1]
        .split(",")
        .map((p) => p.trim())
        .filter(Boolean);
      if (parts.length >= 3) {
        const r = parseFloat(parts[0]);
        const g = parseFloat(parts[1]);
        const b = parseFloat(parts[2]);
        const a = parts.length >= 4 ? parseFloat(parts[3]) : 1;
        return [r / 255, g / 255, b / 255, Number.isFinite(a) ? a : 1];
      }
    }
    return fallback.slice();
  }

  function toOpaqueColor(input, fallback = WEBGL_CLEAR_FALLBACK) {
    const base = Array.isArray(fallback) ? fallback : WEBGL_CLEAR_FALLBACK;
    const rgba = parseColor(input, base);
    const alpha = clamp(Number.isFinite(rgba[3]) ? rgba[3] : 1, 0, 1);
    if (alpha >= 0.999) return [rgba[0], rgba[1], rgba[2], 1];
    const inv = 1 - alpha;
    return [
      rgba[0] * alpha + base[0] * inv,
      rgba[1] * alpha + base[1] * inv,
      rgba[2] * alpha + base[2] * inv,
      1,
    ];
  }

  function mixColor(base, target, t) {
    const b = Array.isArray(base) ? base : parseColor(base);
    const c = Array.isArray(target) ? target : parseColor(target);
    const k = clamp(t, 0, 1);
    return [
      b[0] + (c[0] - b[0]) * k,
      b[1] + (c[1] - b[1]) * k,
      b[2] + (c[2] - b[2]) * k,
      b[3] + (c[3] - b[3]) * k,
    ];
  }

  function resolveDash(edgeDash, lineWidth) {
    const dash = String(edgeDash || "").toLowerCase();
    if (dash === "dash" || dash === "dashed" || dash === "longdash" || dash === "dashdot") {
      const size = Math.max(4, lineWidth * 3.6);
      const gap = Math.max(3, lineWidth * 2.2);
      return { type: 1, size, gap };
    }
    if (dash === "dot" || dash === "dotted") {
      const size = Math.max(1.5, lineWidth * 1.2);
      const gap = Math.max(2.5, lineWidth * 2);
      return { type: 2, size, gap };
    }
    return { type: 0, size: 0, gap: 0 };
  }

  function normalizeVector(x, y) {
    const len = Math.hypot(x, y);
    if (len < EPS) return { x: 1, y: 0, len: 0 };
    return { x: x / len, y: y / len, len };
  }

  function computeEdgeEndpoints(sx, sy, tx, ty, sr, tr, lineWidth, startExtra = 0, endExtra = 0) {
    const dir = normalizeVector(tx - sx, ty - sy);
    if (dir.len < EPS) return { sx, sy, tx, ty, dir };
    const padBase = Math.max(2, lineWidth * 0.6);
    let startPad = Math.max(sr + padBase, sr + 2) + (Number(startExtra) || 0);
    let endPad = Math.max(tr + padBase, tr + 2) + (Number(endExtra) || 0);
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

  function computeArrowLineTrim(arrowBack, lineWidth, sizeScale = 1) {
    const back = Math.max(0, Number(arrowBack) || 0);
    if (back <= EPS) return 0;
    const lw = Math.max(0, Number(lineWidth) || 0);
    const scale = Math.max(0.1, Number(sizeScale) || 1);
    const overlap = Math.min(Math.max(0.95 * scale, lw * 0.72), back * 0.72);
    return Math.max(0, back - overlap);
  }

  function computeLabelSpan(absUx, absUy, w, h) {
    const spanW = absUx > EPS ? w / absUx : Infinity;
    const spanH = absUy > EPS ? h / absUy : Infinity;
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
      if (/\s/.test(ch)) {
        units += 0.33;
      } else if (cp > 255) {
        units += 1.0;
      } else if (/[A-Z0-9]/.test(ch)) {
        units += 0.64;
      } else {
        units += 0.56;
      }
    }
    return Math.max(1, units * Math.max(1, size));
  }

  function estimateTextBox(text, size, sizeScale = 1) {
    if (!text) return { w: 0, h: 0 };
    const scale = Number(sizeScale) || 1;
    return {
      w: Math.max(24, estimateTextWidth(text, size)) * scale,
      h: Math.max(12, size + 6) * scale,
    };
  }

  function truncateTextByWorldWidth(text, size, maxWorldWidth, sizeScale = 1) {
    const raw = String(text || "");
    if (!raw) return "";
    const worldLimit = Number(maxWorldWidth);
    if (!Number.isFinite(worldLimit) || worldLimit <= 0) return raw;
    const scale = Math.max(Number(sizeScale) || 1, EPS);
    const maxWidthPx = worldLimit / scale;
    if (estimateTextWidth(raw, size) <= maxWidthPx + EPS) return raw;
    const chars = Array.from(raw);
    if (!chars.length) return "";
    const ellipsis = TEXT_ELLIPSIS;
    const ellipsisWidth = estimateTextWidth(ellipsis, size);
    if (ellipsisWidth >= maxWidthPx - EPS) return chars[0] ? `${chars[0]}${ellipsis}` : ellipsis;
    let lo = 0;
    let hi = chars.length;
    while (lo < hi) {
      const mid = Math.ceil((lo + hi) / 2);
      const candidate = `${chars.slice(0, mid).join("")}${ellipsis}`;
      if (estimateTextWidth(candidate, size) <= maxWidthPx + EPS) lo = mid;
      else hi = mid - 1;
    }
    const keep = clamp(lo, 1, chars.length);
    return `${chars.slice(0, keep).join("")}${ellipsis}`;
  }

  window.__ANALYTIX_GRAPH_ENGINE_PRIMITIVES__ = {
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
  };
})();
