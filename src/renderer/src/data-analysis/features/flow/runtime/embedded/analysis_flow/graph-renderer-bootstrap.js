/* global window */
(() => {
  const WEBGL_CLEAR_FALLBACK = [246 / 255, 247 / 255, 251 / 255, 1];

  function createGraphCanvas(container, clearColor = WEBGL_CLEAR_FALLBACK) {
    if (!container) return null;
    container.innerHTML = "";
    const canvas = document.createElement("canvas");
    canvas.className = "graphWebglCanvas";
    canvas.style.position = "absolute";
    canvas.style.left = "0";
    canvas.style.top = "0";
    canvas.style.width = "100%";
    canvas.style.height = "100%";
    canvas.style.display = "block";
    canvas.style.backgroundColor = `rgb(${Math.round(clearColor[0] * 255)}, ${Math.round(clearColor[1] * 255)}, ${Math.round(
      clearColor[2] * 255
    )})`;
    container.appendChild(canvas);
    return canvas;
  }

  function createGraphGlContext(canvas, clearColor = WEBGL_CLEAR_FALLBACK) {
    if (!canvas) {
      return {
        gl: null,
        isWebGL2: false,
        instancedExt: null,
        fragPrecision: "mediump",
      };
    }
    const ctxOpts = { preserveDrawingBuffer: true, antialias: true, alpha: false };
    let gl = canvas.getContext("webgl2", ctxOpts);
    let isWebGL2 = true;
    if (!gl) {
      gl = canvas.getContext("webgl", ctxOpts);
      isWebGL2 = false;
    }
    const instancedExt = !isWebGL2 && gl ? gl.getExtension("ANGLE_instanced_arrays") : null;
    if (!gl) {
      return {
        gl: null,
        isWebGL2,
        instancedExt,
        fragPrecision: "mediump",
      };
    }
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
    gl.disable(gl.DITHER);
    gl.clearColor(clearColor[0], clearColor[1], clearColor[2], 1);
    gl.clear(gl.COLOR_BUFFER_BIT);
    const highFloat = gl.getShaderPrecisionFormat ? gl.getShaderPrecisionFormat(gl.FRAGMENT_SHADER, gl.HIGH_FLOAT) : null;
    return {
      gl,
      isWebGL2,
      instancedExt,
      fragPrecision: highFloat && Number(highFloat.precision || 0) > 0 ? "highp" : "mediump",
    };
  }

  window.__ANALYTIX_GRAPH_RENDERER_BOOTSTRAP__ = {
    WEBGL_CLEAR_FALLBACK,
    createGraphCanvas,
    createGraphGlContext,
  };
})();
