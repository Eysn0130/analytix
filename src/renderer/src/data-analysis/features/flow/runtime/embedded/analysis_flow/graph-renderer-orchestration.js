/* global window */
(() => {
  const graphRendererBootstrap = window.__ANALYTIX_GRAPH_RENDERER_BOOTSTRAP__ || null;
  if (!graphRendererBootstrap) {
    throw new Error("graph renderer bootstrap missing for orchestration");
  }
  const { WEBGL_CLEAR_FALLBACK } = graphRendererBootstrap;

  function compileGraphShader(engine, type, source, label = "") {
    const gl = engine?.gl;
    const shader = gl.createShader(type);
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    const compiled = !!gl.getShaderParameter(shader, gl.COMPILE_STATUS);
    const infoLog = gl.getShaderInfoLog(shader) || "";
    if (!compiled) {
      engine?._emitWebglWarn?.(`shader-compile:${label || String(type)}`, "WebGL shader compile failed", infoLog);
    }
    if (label) {
      engine._shaderInfo[label] = { compiled, log: compiled ? "" : infoLog };
    }
    return shader;
  }

  function createGraphProgram(engine, vsSource, fsSource, label = "") {
    const gl = engine?.gl;
    const vs = compileGraphShader(engine, gl.VERTEX_SHADER, vsSource, label ? `${label}:vs` : "");
    const fs = compileGraphShader(engine, gl.FRAGMENT_SHADER, fsSource, label ? `${label}:fs` : "");
    const program = gl.createProgram();
    gl.attachShader(program, vs);
    gl.attachShader(program, fs);
    gl.linkProgram(program);
    const linked = !!gl.getProgramParameter(program, gl.LINK_STATUS);
    const infoLog = gl.getProgramInfoLog(program) || "";
    if (!linked) {
      engine?._emitWebglWarn?.(`program-link:${label || "unknown"}`, "WebGL program link failed", infoLog);
    }
    try {
      if (vs) gl.deleteShader(vs);
    } catch {}
    try {
      if (fs) gl.deleteShader(fs);
    } catch {}
    if (label) {
      engine._programInfo[label] = { linked, log: linked ? "" : infoLog };
    }
    return program;
  }

  function drawGraphInstanced(engine, mode, vertexCount, instanceCount) {
    const gl = engine?.gl;
    if (engine?._isWebGL2 && gl.drawArraysInstanced) {
      gl.drawArraysInstanced(mode, 0, vertexCount, instanceCount);
      return;
    }
    if (engine?._instancedExt && engine._instancedExt.drawArraysInstancedANGLE) {
      engine._instancedExt.drawArraysInstancedANGLE(mode, 0, vertexCount, instanceCount);
      return;
    }
    gl.drawArrays(mode, 0, vertexCount * instanceCount);
  }

  function drawGraphScene(engine, { clearFallback = WEBGL_CLEAR_FALLBACK } = {}) {
    const gl = engine?.gl;
    if (!gl) return;
    const clearColor = engine._clearColor || clearFallback;
    gl.clearColor(clearColor[0], clearColor[1], clearColor[2], 1);
    gl.clear(gl.COLOR_BUFFER_BIT);
    const resolution = [engine.canvas.width, engine.canvas.height];
    const scale = engine._scale * engine._pixelRatio;
    const translate = { x: engine._translate.x * engine._pixelRatio, y: engine._translate.y * engine._pixelRatio };
    const visual = engine._computeZoomVisualStyle();
    const useCull = engine._updateEdgeCullActive();
    let edgeBuffer = engine._edgeBuffer;
    let edgeCount = engine._edgeSegmentsCount;
    let arrowBuffer = engine._arrowBuffer;
    let arrowCount = engine._arrowCount;
    let needRepair = false;
    const repairReasons = [];
    if (useCull) {
      const allowRebuild = !engine._interactionActive && !engine._wheelActive;
      if (engine._edgeViewDirty && allowRebuild) {
        engine._rebuildVisibleEdges();
      }
      const canUseVisible = engine._edgeVisibleCount > 0 && engine._edgeVisibleBuffer && !engine._edgeViewDirty;
      if (canUseVisible) {
        edgeBuffer = engine._edgeVisibleBuffer;
        edgeCount = engine._edgeVisibleCount;
        arrowBuffer = engine._arrowVisibleBuffer || engine._arrowBuffer;
        arrowCount = engine._arrowVisibleCount;
      } else if (engine._edgeSegmentsCount > 0) {
        edgeBuffer = engine._edgeBuffer;
        edgeCount = engine._edgeSegmentsCount;
        arrowBuffer = engine._arrowBuffer;
        arrowCount = engine._arrowCount;
      }
    }

    if (edgeCount > 0) {
      const edgeStride = 12;
      const edgeSourceData = edgeBuffer === engine._edgeVisibleBuffer ? engine._edgeVisibleData : engine._edgeData;
      const edgeAvail = edgeSourceData ? Math.floor(edgeSourceData.length / edgeStride) : 0;
      if (edgeCount > edgeAvail) {
        edgeCount = edgeAvail;
        engine._edgeGeomDirty = true;
        engine._edgeViewDirty = true;
        needRepair = true;
        repairReasons.push("edge-buffer-short");
      }
      gl.useProgram(engine._edgeProgram);
      engine._bindEdgeAttributes(scale, translate, resolution, edgeBuffer, visual.edgeContrast, clearColor);
      drawGraphInstanced(engine, gl.TRIANGLE_STRIP, 4, edgeCount);
    }

    if (engine._highlightDirty) engine._rebuildHighlightBuffers();
    if (engine._highlightEdgeCount > 0 && engine._highlightEdgeBuffer) {
      const edgeStride = 12;
      const avail = engine._highlightEdgeData ? Math.floor(engine._highlightEdgeData.length / edgeStride) : 0;
      const count = Math.min(engine._highlightEdgeCount, avail);
      gl.useProgram(engine._edgeProgram);
      engine._bindEdgeAttributes(scale, translate, resolution, engine._highlightEdgeBuffer, 1, clearColor);
      drawGraphInstanced(engine, gl.TRIANGLE_STRIP, 4, count);
    }

    if (arrowCount > 0) {
      const arrowStride = 9;
      const arrowSourceData = arrowBuffer === engine._arrowVisibleBuffer ? engine._arrowVisibleData : engine._arrowData;
      const arrowAvail = arrowSourceData ? Math.floor(arrowSourceData.length / arrowStride) : 0;
      if (arrowCount > arrowAvail) {
        arrowCount = arrowAvail;
        engine._edgeGeomDirty = true;
        engine._edgeViewDirty = true;
        needRepair = true;
        repairReasons.push("arrow-buffer-short");
      }
      gl.useProgram(engine._arrowProgram);
      engine._bindArrowAttributes(scale, translate, resolution, arrowBuffer, visual.edgeContrast, clearColor);
      drawGraphInstanced(engine, gl.TRIANGLES, 3, arrowCount);
    }

    if (engine._shadowCount > 0) {
      const nodeStride = 15;
      const shadowAvail = engine._shadowData ? Math.floor(engine._shadowData.length / nodeStride) : 0;
      const shadowCount = Math.min(engine._shadowCount, shadowAvail);
      gl.useProgram(engine._nodeProgram);
      engine._bindNodeAttributes(scale, translate, resolution, true, 1, clearColor);
      drawGraphInstanced(engine, gl.TRIANGLE_STRIP, 4, shadowCount);
    }

    if (engine._nodeCount > 0) {
      const nodeStride = 15;
      const nodeAvail = engine._nodeData ? Math.floor(engine._nodeData.length / nodeStride) : 0;
      const nodeCount = Math.min(engine._nodeCount, nodeAvail);
      if (nodeCount < engine._nodeCount) {
        engine._positionsDirty = true;
        needRepair = true;
        repairReasons.push("node-buffer-short");
      }
      gl.useProgram(engine._nodeProgram);
      engine._bindNodeAttributes(scale, translate, resolution, false, visual.nodeContrast, clearColor);
      drawGraphInstanced(engine, gl.TRIANGLE_STRIP, 4, nodeCount);
    }
    if (needRepair) {
      engine._renderRepairCount += 1;
      engine._renderRepairReason = repairReasons.join(",");
      engine._renderRepairAt = Date.now();
      engine._scheduleRender();
    }
  }

  window.__ANALYTIX_GRAPH_RENDERER_ORCHESTRATION__ = {
    compileGraphShader,
    createGraphProgram,
    drawGraphInstanced,
    drawGraphScene,
  };
})();
