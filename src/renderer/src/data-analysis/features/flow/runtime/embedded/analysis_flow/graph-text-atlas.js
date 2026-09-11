/* global window */
(() => {
  const graphEnginePrimitives = window.__ANALYTIX_GRAPH_ENGINE_PRIMITIVES__ || null;
  if (!graphEnginePrimitives) {
    throw new Error("graph engine primitives missing for text atlas");
  }
  const { clamp } = graphEnginePrimitives;

  const DEFAULT_FONT_FAMILY =
    '"Source Han Sans SC","Noto Sans SC","PingFang SC","Microsoft YaHei",ui-sans-serif,system-ui';
  const TEXT_BASE_SIZE = 32;
  const TEXT_ATLAS_PADDING = 4;
  const TEXT_ATLAS_SIZE = 2048;
  const TEXT_ATLAS_SCALE = 4;
  const TEXT_ATLAS_UV_INSET = 0.5;
  const TEXT_ATLAS_SET_CACHE_LIMIT = 24;
  const TEXT_ATLAS_CACHE_LIMIT_BYTES = 64 * 1024 * 1024;
  const TEXT_ATLAS_IDLE_EVICT_MS = 60 * 1000;

  class TextAtlas {
    constructor(gl, opts = {}) {
      this.gl = gl;
      this.baseSize = Number(opts.baseSize) || TEXT_BASE_SIZE;
      this.scale = Number(opts.scale) || 1;
      this.fontFamily = opts.fontFamily || DEFAULT_FONT_FAMILY;
      this.fontWeight = opts.fontWeight || 500;
      this.fontStyle = opts.fontStyle || "normal";
      this.padding = Number.isFinite(opts.padding) ? opts.padding : TEXT_ATLAS_PADDING;
      this.size = Number(opts.size) || TEXT_ATLAS_SIZE;
      this.maxSize = Number(opts.maxSize) || this.size;
      this.canvas = document.createElement("canvas");
      this.canvas.width = this.size;
      this.canvas.height = this.size;
      this.ctx = this.canvas.getContext("2d");
      this.lastUsedAt = typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
      if (!this.ctx) {
        this.canvas = null;
        this.texture = null;
        this.glyphs = new Map();
        this._whiteGlyph = null;
        this.ascent = this.baseSize * 0.8;
        this.descent = this.baseSize * 0.2;
        return;
      }
      this.ctx.clearRect(0, 0, this.size, this.size);
      this.ctx.fillStyle = "#fff";
      this.ctx.textBaseline = "alphabetic";
      this.ctx.textAlign = "left";
      this.ctx.imageSmoothingEnabled = true;
      this.ctx.imageSmoothingQuality = "high";
      const fontPx = this.baseSize * this.scale;
      this.ctx.font = `${this.fontStyle} ${this.fontWeight} ${fontPx}px ${this.fontFamily}`;
      this.cursorX = this.padding;
      this.cursorY = this.padding;
      this.rowH = 0;
      this.glyphs = new Map();
      this.dirty = true;
      this.texture = null;
      this._whiteGlyph = null;

      const metrics = this.ctx.measureText("Hg");
      const ascentPx = metrics.actualBoundingBoxAscent || fontPx * 0.8;
      const descentPx = metrics.actualBoundingBoxDescent || fontPx * 0.2;
      const logicalScale = 1 / this.scale;
      this.ascent = ascentPx * logicalScale;
      this.descent = descentPx * logicalScale;
    }

    _touch() {
      this.lastUsedAt = typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
    }

    _ensureTexture() {
      if (this.texture || !this.gl) return;
      this._touch();
      const gl = this.gl;
      const tex = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, tex);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.ALPHA, this.size, this.size, 0, gl.ALPHA, gl.UNSIGNED_BYTE, null);
      this.texture = tex;
      this.dirty = true;
    }

    _flushTexture() {
      if (!this.gl || !this.texture || !this.dirty) return;
      const gl = this.gl;
      gl.bindTexture(gl.TEXTURE_2D, this.texture);
      gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.ALPHA, gl.ALPHA, gl.UNSIGNED_BYTE, this.canvas);
      this.dirty = false;
    }

    _nextRow() {
      this.cursorX = this.padding;
      this.cursorY += this.rowH + this.padding;
      this.rowH = 0;
    }

    canFit(w, h) {
      if (this.cursorY + h + this.padding > this.size) return false;
      if (this.cursorX + w + this.padding > this.size) {
        return this.cursorY + h + this.padding + this.rowH <= this.size;
      }
      return true;
    }

    _allocRect(w, h) {
      if (this.cursorX + w + this.padding > this.size) {
        this._nextRow();
      }
      if (this.cursorY + h + this.padding > this.size) return null;
      const x = this.cursorX;
      const y = this.cursorY;
      this.cursorX += w + this.padding;
      this.rowH = Math.max(this.rowH, h);
      return { x, y };
    }

    _drawGlyph(ch) {
      const ctx = this.ctx;
      if (!ctx) return null;
      const fontPx = this.baseSize * this.scale;
      ctx.textAlign = "left";
      ctx.textBaseline = "alphabetic";
      ctx.font = `${this.fontStyle} ${this.fontWeight} ${fontPx}px ${this.fontFamily}`;
      const metrics = ctx.measureText(ch);
      const measuredW = metrics.width || 0;
      const boundL = metrics.actualBoundingBoxLeft || 0;
      const boundR = metrics.actualBoundingBoxRight || measuredW || this.baseSize * 0.6;
      const ascentPx = metrics.actualBoundingBoxAscent || this.ascent * this.scale;
      const descentPx = metrics.actualBoundingBoxDescent || this.descent * this.scale;
      const advanceW = Math.max(measuredW, this.baseSize * 0.5);
      const w = Math.ceil(Math.max(advanceW, boundL + boundR, this.baseSize * 0.5));
      const h = Math.ceil(ascentPx + descentPx);
      const boxW = Math.max(1, w + this.padding * 2);
      const boxH = Math.max(1, h + this.padding * 2);
      const rect = this._allocRect(boxW, boxH);
      if (!rect) return null;
      const contentX = rect.x + this.padding;
      const contentY = rect.y + this.padding;
      const x = Math.round(contentX);
      const y = Math.round(contentY + ascentPx);
      ctx.fillStyle = "#fff";
      ctx.fillText(ch, x, y);
      const fallbackAdvance = Math.max(1, advanceW || w || this.baseSize * 0.6);
      const logicalScale = 1 / this.scale;
      const offsetX = (x - contentX) * logicalScale;
      const offsetY = (y - (contentY + ascentPx)) * logicalScale;
      const visCx = (x - contentX) + (-boundL + boundR) * 0.5;
      const visCy = (y - contentY) + (-ascentPx + descentPx) * 0.5;
      const centerX = w * 0.5;
      const centerY = h * 0.5;
      const iconOffsetX = clamp((centerX - visCx) * logicalScale, -this.baseSize * 0.35 * logicalScale, this.baseSize * 0.35 * logicalScale);
      const iconOffsetY = clamp((centerY - visCy) * logicalScale, -this.baseSize * 0.35 * logicalScale, this.baseSize * 0.35 * logicalScale);
      const glyph = {
        x: rect.x,
        y: rect.y,
        w: boxW,
        h: boxH,
        contentX,
        contentY,
        contentW: w,
        contentH: h,
        width: w * logicalScale,
        height: h * logicalScale,
        advance: fallbackAdvance * logicalScale,
        bearingX: 0,
        bearingY: (y - contentY) * logicalScale,
        offsetX,
        offsetY,
        iconOffsetX,
        iconOffsetY,
        scale: logicalScale,
      };
      this.dirty = true;
      return glyph;
    }

    _drawImageGlyph(img, width, height) {
      if (!this.ctx) return null;
      if (!img || !width || !height) return null;
      const scale = this.scale;
      const texW = Math.max(1, Math.ceil(width * scale));
      const texH = Math.max(1, Math.ceil(height * scale));
      const boxW = Math.max(1, texW + this.padding * 2);
      const boxH = Math.max(1, texH + this.padding * 2);
      const rect = this._allocRect(boxW, boxH);
      if (!rect) return null;
      const x = rect.x + this.padding;
      const y = rect.y + this.padding;
      this.ctx.drawImage(img, x, y, texW, texH);
      const glyph = {
        x: rect.x,
        y: rect.y,
        w: boxW,
        h: boxH,
        contentX: x,
        contentY: y,
        contentW: texW,
        contentH: texH,
        width,
        height,
        advance: width,
        bearingX: 0,
        bearingY: height,
        scale: 1,
      };
      this.dirty = true;
      return glyph;
    }

    getImageGlyph(img, width, height, key = "") {
      if (!img) return null;
      this._touch();
      const cacheKey = `__img:${key || ""}:${width}x${height}`;
      if (this.glyphs.has(cacheKey)) return this.glyphs.get(cacheKey);
      const glyph = this._drawImageGlyph(img, width, height);
      if (!glyph) return null;
      const cx = Number.isFinite(glyph.contentX) ? glyph.contentX : glyph.x + this.padding;
      const cy = Number.isFinite(glyph.contentY) ? glyph.contentY : glyph.y + this.padding;
      const contentW = Math.max(1, Number(glyph.contentW) || glyph.w - this.padding * 2);
      const contentH = Math.max(1, Number(glyph.contentH) || glyph.h - this.padding * 2);
      const insetX = contentW > 2 ? TEXT_ATLAS_UV_INSET : 0;
      const insetY = contentH > 2 ? TEXT_ATLAS_UV_INSET : 0;
      const uv = {
        u0: (cx + insetX) / this.size,
        v0: (cy + insetY) / this.size,
        u1: (cx + contentW - insetX) / this.size,
        v1: (cy + contentH - insetY) / this.size,
      };
      const out = { ...glyph, ...uv };
      this.glyphs.set(cacheKey, out);
      return out;
    }

    getGlyph(ch) {
      this._touch();
      if (this.glyphs.has(ch)) return this.glyphs.get(ch);
      const glyph = this._drawGlyph(ch);
      if (!glyph) return null;
      const cx = Number.isFinite(glyph.contentX) ? glyph.contentX : glyph.x + this.padding;
      const cy = Number.isFinite(glyph.contentY) ? glyph.contentY : glyph.y + this.padding;
      const contentW = Math.max(1, Number(glyph.contentW) || glyph.w - this.padding * 2);
      const contentH = Math.max(1, Number(glyph.contentH) || glyph.h - this.padding * 2);
      const insetX = contentW > 2 ? TEXT_ATLAS_UV_INSET : 0;
      const insetY = contentH > 2 ? TEXT_ATLAS_UV_INSET : 0;
      const uv = {
        u0: (cx + insetX) / this.size,
        v0: (cy + insetY) / this.size,
        u1: (cx + contentW - insetX) / this.size,
        v1: (cy + contentH - insetY) / this.size,
      };
      const out = { ...glyph, ...uv };
      this.glyphs.set(ch, out);
      return out;
    }

    getWhiteGlyph() {
      this._touch();
      if (!this.ctx) return null;
      if (this._whiteGlyph) return this._whiteGlyph;
      const rect = this._allocRect(4, 4);
      if (!rect) return null;
      const logicalScale = 1 / this.scale;
      this.ctx.fillStyle = "#fff";
      this.ctx.fillRect(rect.x, rect.y, 4, 4);
      this.dirty = true;
      const inset = Math.min(1, this.padding * 0.5);
      this._whiteGlyph = {
        x: rect.x,
        y: rect.y,
        w: 4,
        h: 4,
        width: 4 * logicalScale,
        height: 4 * logicalScale,
        advance: 4 * logicalScale,
        bearingX: 0,
        bearingY: 0,
        u0: (rect.x + inset) / this.size,
        v0: (rect.y + inset) / this.size,
        u1: (rect.x + 4 - inset) / this.size,
        v1: (rect.y + 4 - inset) / this.size,
      };
      return this._whiteGlyph;
    }

    getGlyphCount() {
      return (this.glyphs ? this.glyphs.size : 0) + (this._whiteGlyph ? 1 : 0);
    }

    getApproxTextureBytes() {
      return Math.max(0, Number(this.size) || 0) * Math.max(0, Number(this.size) || 0);
    }

    destroy() {
      const gl = this.gl;
      if (gl && this.texture) {
        try {
          gl.deleteTexture(this.texture);
        } catch (e) {}
      }
      this.texture = null;
      if (this.glyphs) this.glyphs.clear();
      this._whiteGlyph = null;
      this.ctx = null;
      this.canvas = null;
      this.gl = null;
    }
  }

  class TextAtlasSet {
    constructor(gl, opts = {}) {
      this.gl = gl;
      this.fontFamily = opts.fontFamily || DEFAULT_FONT_FAMILY;
      this.fontWeight = opts.fontWeight || 500;
      this.fontStyle = opts.fontStyle || "normal";
      this.baseSize = Number(opts.baseSize) || TEXT_BASE_SIZE;
      this.scale = Number(opts.scale) || 1;
      this.maxSize = Number(opts.maxSize) || TEXT_ATLAS_SIZE;
      this._atlasOptions = {
        fontFamily: this.fontFamily,
        fontWeight: this.fontWeight,
        fontStyle: this.fontStyle,
        baseSize: this.baseSize,
        scale: this.scale,
        size: this.maxSize,
        maxSize: this.maxSize,
        padding: Number.isFinite(opts.padding) ? Number(opts.padding) : TEXT_ATLAS_PADDING,
      };
      this.lastUsedAt = typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
      this.atlases = [this._createAtlas()];
      this.glyphs = new Map();
    }

    _touch() {
      this.lastUsedAt = typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
    }

    _createAtlas() {
      return new TextAtlas(this.gl, this._atlasOptions);
    }

    _acquireAtlas(wantW, wantH) {
      let atlas = this.atlases[this.atlases.length - 1];
      if (!atlas || !atlas.canFit(wantW, wantH)) {
        atlas = this._createAtlas();
        this.atlases.push(atlas);
      }
      return atlas;
    }

    getGlyph(ch) {
      this._touch();
      if (this.glyphs.has(ch)) return this.glyphs.get(ch);
      let atlas = this._acquireAtlas(this.baseSize * 1.5, this.baseSize * 1.5);
      let glyph = atlas.getGlyph(ch);
      if (!glyph) {
        atlas = this._createAtlas();
        this.atlases.push(atlas);
        glyph = atlas.getGlyph(ch);
      }
      if (!glyph) return null;
      const out = { ...glyph, atlas };
      this.glyphs.set(ch, out);
      return out;
    }

    getWhiteGlyph() {
      this._touch();
      const atlas = this.atlases[0];
      const glyph = atlas.getWhiteGlyph();
      return glyph ? { ...glyph, atlas } : null;
    }

    getImageGlyph(key, img, width, height) {
      this._touch();
      const cacheKey = `__img:${key}:${width}x${height}`;
      if (this.glyphs.has(cacheKey)) return this.glyphs.get(cacheKey);
      const wantW = Math.max(width, this.baseSize) * 1.6;
      let atlas = this._acquireAtlas(wantW, wantW);
      let glyph = atlas.getImageGlyph(img, width, height, cacheKey);
      if (!glyph) {
        atlas = this._createAtlas();
        this.atlases.push(atlas);
        glyph = atlas.getImageGlyph(img, width, height, cacheKey);
      }
      if (!glyph) return null;
      const out = { ...glyph, atlas };
      this.glyphs.set(cacheKey, out);
      return out;
    }

    flush() {
      this._touch();
      this.atlases.forEach((atlas) => {
        atlas._ensureTexture();
        atlas._flushTexture();
      });
    }

    hasActiveAtlas(activeAtlases) {
      if (!(activeAtlases instanceof Set) || !activeAtlases.size) return false;
      return this.atlases.some((atlas) => activeAtlases.has(atlas));
    }

    isIdle(now, idleMs) {
      return now - this.lastUsedAt >= idleMs;
    }

    getStats() {
      const atlasCount = Array.isArray(this.atlases) ? this.atlases.length : 0;
      const glyphCount = (Array.isArray(this.atlases) ? this.atlases : []).reduce(
        (sum, atlas) => sum + (atlas?.getGlyphCount?.() || 0),
        0
      );
      const approxTextureBytes = (Array.isArray(this.atlases) ? this.atlases : []).reduce(
        (sum, atlas) => sum + (atlas?.getApproxTextureBytes?.() || 0),
        0
      );
      return {
        atlasCount,
        glyphCount,
        approxTextureBytes,
        lastUsedAt: this.lastUsedAt,
      };
    }

    destroy() {
      this.glyphs.clear();
      this.atlases.forEach((atlas) => {
        try {
          atlas?.destroy?.();
        } catch (e) {}
      });
      this.atlases = [];
      this.gl = null;
    }
  }

  window.__ANALYTIX_GRAPH_TEXT_ATLAS__ = {
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
  };
})();
