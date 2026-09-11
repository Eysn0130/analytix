/* global window */
(() => {
  const rendererBootstrap = window.__ANALYTIX_GRAPH_RENDERER_BOOTSTRAP__ || null;
  if (!rendererBootstrap) {
    throw new Error("graph renderer bootstrap missing for text render pass");
  }
  const { WEBGL_CLEAR_FALLBACK } = rendererBootstrap;

  function drawGraphTextBatches(engine) {
    if (!engine._shouldUseGpuText() || !engine.gl || !engine._textProgram) return;
    if (engine._textDirty) engine._buildTextBatches();
    if (!engine._textBatches.length) return;
    const gl = engine.gl;
    const resolution = [engine.canvas.width, engine.canvas.height];
    const scale = engine._scale * engine._pixelRatio;
    const translate = { x: engine._translate.x * engine._pixelRatio, y: engine._translate.y * engine._pixelRatio };
    const visual = engine._computeZoomVisualStyle();
    const clearColor = engine._clearColor || WEBGL_CLEAR_FALLBACK;
    gl.useProgram(engine._textProgram);
    engine._textBatches.forEach((batch) => {
      if (!batch || !batch.buffer || !batch.atlas) return;
      batch.atlas._ensureTexture();
      batch.atlas._flushTexture();
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, batch.atlas.texture);
      engine._bindTextAttributes(scale, translate, resolution, batch, visual.labelContrast, clearColor, visual.labelAlpha);
      engine._drawInstanced(gl.TRIANGLE_STRIP, 4, batch.count);
    });
  }

  window.__ANALYTIX_GRAPH_TEXT_RENDER_PASS__ = {
    drawGraphTextBatches,
  };
})();
