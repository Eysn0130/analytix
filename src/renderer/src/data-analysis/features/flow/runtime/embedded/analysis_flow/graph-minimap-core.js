/* global window */
(() => {
  class Minimap {
    constructor(opts = {}) {
      this.container = opts.container || null;
      this.size = Array.isArray(opts.size) ? opts.size : [220, 140];
      this.delegateStyle = opts.delegateStyle || {};
      this.contentPaddingRatio = Number.isFinite(opts.contentPaddingRatio)
        ? Math.max(0, Number(opts.contentPaddingRatio))
        : 0.16;
      this.viewportMarginFactor = Number.isFinite(opts.viewportMarginFactor)
        ? Math.max(1.05, Number(opts.viewportMarginFactor))
        : 1.18;
      this.maxNodeSamples = Number.isFinite(opts.maxNodeSamples)
        ? Math.max(80, Math.floor(Number(opts.maxNodeSamples)))
        : 1800;
      this.maxEdgeSamples = Number.isFinite(opts.maxEdgeSamples)
        ? Math.max(120, Math.floor(Number(opts.maxEdgeSamples)))
        : 2400;
      this.graph = null;
      this.canvas = null;
      this.ctx = null;
      this._bounds = null;
    }

    init(graph) {
      this.graph = graph;
      if (!this.container) return;
      this.container.innerHTML = "";
      const canvas = document.createElement("canvas");
      canvas.width = this.size[0];
      canvas.height = this.size[1];
      canvas.style.width = `${this.size[0]}px`;
      canvas.style.height = `${this.size[1]}px`;
      canvas.style.display = "block";
      this.container.appendChild(canvas);
      this.canvas = canvas;
      this.ctx = canvas.getContext("2d");
    }

    destroy() {
      if (this.container) this.container.innerHTML = "";
      this.canvas = null;
      this.ctx = null;
      this.graph = null;
      this._bounds = null;
    }

    _computeBounds(nodes) {
      if (!nodes.length) return null;
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      nodes.forEach((node) => {
        const m = node.getModel ? node.getModel() : node;
        if (!m) return;
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

    _sampleStep(total, maxSamples) {
      if (!Number.isFinite(total) || total <= 0) return 1;
      if (!Number.isFinite(maxSamples) || maxSamples <= 0 || total <= maxSamples) return 1;
      return Math.max(1, Math.ceil(total / maxSamples));
    }

    _expandBounds(bounds, view) {
      if (!bounds) return null;
      let minX = Number(bounds.minX);
      let maxX = Number(bounds.maxX);
      let minY = Number(bounds.minY);
      let maxY = Number(bounds.maxY);
      if (!Number.isFinite(minX) || !Number.isFinite(maxX) || !Number.isFinite(minY) || !Number.isFinite(maxY)) {
        return null;
      }
      const viewX = Number(view?.x);
      const viewY = Number(view?.y);
      const viewW = Number(view?.width);
      const viewH = Number(view?.height);
      if (Number.isFinite(viewX) && Number.isFinite(viewY) && Number.isFinite(viewW) && Number.isFinite(viewH)) {
        minX = Math.min(minX, viewX);
        minY = Math.min(minY, viewY);
        maxX = Math.max(maxX, viewX + Math.max(1, viewW));
        maxY = Math.max(maxY, viewY + Math.max(1, viewH));
      }
      const spanX = Math.max(1, maxX - minX);
      const spanY = Math.max(1, maxY - minY);
      const viewTargetX =
        Number.isFinite(viewW) && viewW > 0 ? viewW * this.viewportMarginFactor : spanX;
      const viewTargetY =
        Number.isFinite(viewH) && viewH > 0 ? viewH * this.viewportMarginFactor : spanY;
      const targetSpanX = Math.max(spanX * (1 + this.contentPaddingRatio * 2), viewTargetX);
      const targetSpanY = Math.max(spanY * (1 + this.contentPaddingRatio * 2), viewTargetY);
      const cx = (minX + maxX) / 2;
      const cy = (minY + maxY) / 2;
      return {
        minX: cx - targetSpanX / 2,
        maxX: cx + targetSpanX / 2,
        minY: cy - targetSpanY / 2,
        maxY: cy + targetSpanY / 2,
      };
    }

    getBounds() {
      if (!this._bounds) return null;
      return { ...this._bounds };
    }

    updateCanvas() {
      if (!this.ctx || !this.graph) return;
      const nodes = this.graph.getNodes ? this.graph.getNodes() : [];
      const edges = this.graph.getEdges ? this.graph.getEdges() : [];
      const rawBounds = this._computeBounds(nodes);
      const view = this.graph.getViewBox ? this.graph.getViewBox() : null;
      const bounds = this._expandBounds(rawBounds, view);
      this._bounds = bounds;
      const ctx = this.ctx;
      const w = this.canvas.width;
      const h = this.canvas.height;
      ctx.clearRect(0, 0, w, h);
      if (!bounds) return;
      const spanX = bounds.maxX - bounds.minX || 1;
      const spanY = bounds.maxY - bounds.minY || 1;
      const scale = Math.min(w / spanX, h / spanY);
      const ox = (w - spanX * scale) / 2 - bounds.minX * scale;
      const oy = (h - spanY * scale) / 2 - bounds.minY * scale;
      ctx.save();
      ctx.translate(ox, oy);
      ctx.scale(scale, scale);
      ctx.lineWidth = Math.max(1 / scale, 0.35);
      ctx.strokeStyle = "rgba(2,6,23,0.3)";
      ctx.beginPath();
      const edgeStep = this._sampleStep(edges.length, this.maxEdgeSamples);
      for (let i = 0; i < edges.length; i += edgeStep) {
        const edge = edges[i];
        const m = edge?.getModel ? edge.getModel() : edge;
        if (!m) continue;
        const source = this.graph.findById ? this.graph.findById(m.source) : null;
        const target = this.graph.findById ? this.graph.findById(m.target) : null;
        const sm = source?.getModel ? source.getModel() : source;
        const tm = target?.getModel ? target.getModel() : target;
        if (!sm || !tm) continue;
        const sx = Number(sm.x) || 0;
        const sy = Number(sm.y) || 0;
        const tx = Number(tm.x) || 0;
        const ty = Number(tm.y) || 0;
        ctx.moveTo(sx, sy);
        ctx.lineTo(tx, ty);
      }
      ctx.stroke();
      ctx.fillStyle = "rgba(31,111,235,0.65)";
      const nodeStep = this._sampleStep(nodes.length, this.maxNodeSamples);
      for (let i = 0; i < nodes.length; i += nodeStep) {
        const node = nodes[i];
        const m = node.getModel ? node.getModel() : node;
        const x = Number(m.x) || 0;
        const y = Number(m.y) || 0;
        ctx.beginPath();
        ctx.arc(x, y, Math.max(1.2 / scale, 0.6), 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.restore();
    }

    updateViewport() {
      if (!this.ctx || !this.graph || !this._bounds) return;
      const view = this.graph.getViewBox();
      if (!view) return;
      if (
        view.x < this._bounds.minX ||
        view.y < this._bounds.minY ||
        view.x + view.width > this._bounds.maxX ||
        view.y + view.height > this._bounds.maxY
      ) {
        this.updateCanvas();
        if (!this._bounds) return;
      }
      const { minX, maxX, minY, maxY } = this._bounds;
      const w = this.canvas.width;
      const h = this.canvas.height;
      const spanX = maxX - minX || 1;
      const spanY = maxY - minY || 1;
      const scale = Math.min(w / spanX, h / spanY);
      const ox = (w - spanX * scale) / 2 - minX * scale;
      const oy = (h - spanY * scale) / 2 - minY * scale;
      const ctx = this.ctx;
      ctx.save();
      ctx.strokeStyle = this.delegateStyle.stroke || "rgba(31,111,235,0.85)";
      ctx.lineWidth = (this.delegateStyle.lineWidth || 2) * (window.devicePixelRatio || 1);
      ctx.fillStyle = this.delegateStyle.fill || "rgba(31,111,235,0.12)";
      ctx.beginPath();
      const x = ox + view.x * scale;
      const y = oy + view.y * scale;
      const vw = view.width * scale;
      const vh = view.height * scale;
      ctx.rect(x, y, vw, vh);
      ctx.fill();
      ctx.stroke();
      ctx.restore();
    }
  }

  window.__ANALYTIX_GRAPH_MINIMAP_CORE__ = {
    Minimap,
  };
})();
