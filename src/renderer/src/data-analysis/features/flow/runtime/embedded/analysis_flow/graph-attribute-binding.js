/* global window */
(() => {
  const primitives = window.__ANALYTIX_GRAPH_ENGINE_PRIMITIVES__ || null;
  const rendererBootstrap = window.__ANALYTIX_GRAPH_RENDERER_BOOTSTRAP__ || null;
  if (!primitives) {
    throw new Error("graph engine primitives missing for attribute binding");
  }
  if (!rendererBootstrap) {
    throw new Error("graph renderer bootstrap missing for attribute binding");
  }
  const { clamp } = primitives;
  const { WEBGL_CLEAR_FALLBACK } = rendererBootstrap;

  function bindEdgeAttributes(engine, scale, translate, resolution, bufferOverride, contrast = 1, clearColor = null) {
    const gl = engine.gl;
    const program = engine._edgeProgram;
    const edgeStride = 12;
    const unitLoc = gl.getAttribLocation(program, "a_unit");
    const startLoc = gl.getAttribLocation(program, "a_start");
    const endLoc = gl.getAttribLocation(program, "a_end");
    const widthLoc = gl.getAttribLocation(program, "a_width");
    const colorLoc = gl.getAttribLocation(program, "a_color");
    const dashTypeLoc = gl.getAttribLocation(program, "a_dashType");
    const dashSizeLoc = gl.getAttribLocation(program, "a_dashSize");
    const dashGapLoc = gl.getAttribLocation(program, "a_dashGap");

    gl.bindBuffer(gl.ARRAY_BUFFER, engine._edgeUnitBuffer);
    gl.enableVertexAttribArray(unitLoc);
    gl.vertexAttribPointer(unitLoc, 2, gl.FLOAT, false, 0, 0);

    gl.bindBuffer(gl.ARRAY_BUFFER, bufferOverride || engine._edgeBuffer);
    const strideBytes = edgeStride * 4;
    gl.enableVertexAttribArray(startLoc);
    gl.vertexAttribPointer(startLoc, 2, gl.FLOAT, false, strideBytes, 0);
    gl.enableVertexAttribArray(endLoc);
    gl.vertexAttribPointer(endLoc, 2, gl.FLOAT, false, strideBytes, 8);
    gl.enableVertexAttribArray(widthLoc);
    gl.vertexAttribPointer(widthLoc, 1, gl.FLOAT, false, strideBytes, 16);
    gl.enableVertexAttribArray(colorLoc);
    gl.vertexAttribPointer(colorLoc, 4, gl.FLOAT, false, strideBytes, 20);
    gl.enableVertexAttribArray(dashTypeLoc);
    gl.vertexAttribPointer(dashTypeLoc, 1, gl.FLOAT, false, strideBytes, 36);
    gl.enableVertexAttribArray(dashSizeLoc);
    gl.vertexAttribPointer(dashSizeLoc, 1, gl.FLOAT, false, strideBytes, 40);
    gl.enableVertexAttribArray(dashGapLoc);
    gl.vertexAttribPointer(dashGapLoc, 1, gl.FLOAT, false, strideBytes, 44);

    if (engine._isWebGL2) {
      gl.vertexAttribDivisor(unitLoc, 0);
      gl.vertexAttribDivisor(startLoc, 1);
      gl.vertexAttribDivisor(endLoc, 1);
      gl.vertexAttribDivisor(widthLoc, 1);
      gl.vertexAttribDivisor(colorLoc, 1);
      gl.vertexAttribDivisor(dashTypeLoc, 1);
      gl.vertexAttribDivisor(dashSizeLoc, 1);
      gl.vertexAttribDivisor(dashGapLoc, 1);
    } else if (engine._instancedExt) {
      engine._instancedExt.vertexAttribDivisorANGLE(unitLoc, 0);
      engine._instancedExt.vertexAttribDivisorANGLE(startLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(endLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(widthLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(colorLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(dashTypeLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(dashSizeLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(dashGapLoc, 1);
    }

    gl.uniform2f(gl.getUniformLocation(program, "u_resolution"), resolution[0], resolution[1]);
    gl.uniform1f(gl.getUniformLocation(program, "u_scale"), scale);
    gl.uniform2f(gl.getUniformLocation(program, "u_translate"), translate.x, translate.y);
    const bg = Array.isArray(clearColor) ? clearColor : engine._clearColor || WEBGL_CLEAR_FALLBACK;
    gl.uniform1f(gl.getUniformLocation(program, "u_contrast"), clamp(Number(contrast) || 1, 0, 1));
    gl.uniform3f(gl.getUniformLocation(program, "u_clearColor"), bg[0], bg[1], bg[2]);
  }

  function bindNodeAttributes(engine, scale, translate, resolution, useShadow, contrast = 1, clearColor = null) {
    const gl = engine.gl;
    const program = engine._nodeProgram;
    const nodeStride = 15;
    const unitLoc = gl.getAttribLocation(program, "a_unit");
    const centerLoc = gl.getAttribLocation(program, "a_center");
    const radiusLoc = gl.getAttribLocation(program, "a_radius");
    const widthLoc = gl.getAttribLocation(program, "a_lineWidth");
    const strokeLoc = gl.getAttribLocation(program, "a_stroke");
    const fillLoc = gl.getAttribLocation(program, "a_fill");
    const dashTypeLoc = gl.getAttribLocation(program, "a_dashType");
    const dashSizeLoc = gl.getAttribLocation(program, "a_dashSize");
    const dashGapLoc = gl.getAttribLocation(program, "a_dashGap");

    gl.bindBuffer(gl.ARRAY_BUFFER, engine._nodeUnitBuffer);
    gl.enableVertexAttribArray(unitLoc);
    gl.vertexAttribPointer(unitLoc, 2, gl.FLOAT, false, 0, 0);

    gl.bindBuffer(gl.ARRAY_BUFFER, useShadow ? engine._shadowBuffer : engine._nodeBuffer);
    const strideBytes = nodeStride * 4;
    gl.enableVertexAttribArray(centerLoc);
    gl.vertexAttribPointer(centerLoc, 2, gl.FLOAT, false, strideBytes, 0);
    gl.enableVertexAttribArray(radiusLoc);
    gl.vertexAttribPointer(radiusLoc, 1, gl.FLOAT, false, strideBytes, 8);
    gl.enableVertexAttribArray(widthLoc);
    gl.vertexAttribPointer(widthLoc, 1, gl.FLOAT, false, strideBytes, 12);
    gl.enableVertexAttribArray(strokeLoc);
    gl.vertexAttribPointer(strokeLoc, 4, gl.FLOAT, false, strideBytes, 16);
    gl.enableVertexAttribArray(fillLoc);
    gl.vertexAttribPointer(fillLoc, 4, gl.FLOAT, false, strideBytes, 32);
    gl.enableVertexAttribArray(dashTypeLoc);
    gl.vertexAttribPointer(dashTypeLoc, 1, gl.FLOAT, false, strideBytes, 48);
    gl.enableVertexAttribArray(dashSizeLoc);
    gl.vertexAttribPointer(dashSizeLoc, 1, gl.FLOAT, false, strideBytes, 52);
    gl.enableVertexAttribArray(dashGapLoc);
    gl.vertexAttribPointer(dashGapLoc, 1, gl.FLOAT, false, strideBytes, 56);

    if (engine._isWebGL2) {
      gl.vertexAttribDivisor(unitLoc, 0);
      gl.vertexAttribDivisor(centerLoc, 1);
      gl.vertexAttribDivisor(radiusLoc, 1);
      gl.vertexAttribDivisor(widthLoc, 1);
      gl.vertexAttribDivisor(strokeLoc, 1);
      gl.vertexAttribDivisor(fillLoc, 1);
      gl.vertexAttribDivisor(dashTypeLoc, 1);
      gl.vertexAttribDivisor(dashSizeLoc, 1);
      gl.vertexAttribDivisor(dashGapLoc, 1);
    } else if (engine._instancedExt) {
      engine._instancedExt.vertexAttribDivisorANGLE(unitLoc, 0);
      engine._instancedExt.vertexAttribDivisorANGLE(centerLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(radiusLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(widthLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(strokeLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(fillLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(dashTypeLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(dashSizeLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(dashGapLoc, 1);
    }

    gl.uniform2f(gl.getUniformLocation(program, "u_resolution"), resolution[0], resolution[1]);
    gl.uniform1f(gl.getUniformLocation(program, "u_scale"), scale);
    gl.uniform2f(gl.getUniformLocation(program, "u_translate"), translate.x, translate.y);
    const bg = Array.isArray(clearColor) ? clearColor : engine._clearColor || WEBGL_CLEAR_FALLBACK;
    gl.uniform1f(gl.getUniformLocation(program, "u_contrast"), Math.max(0, Number(contrast) || 1));
    gl.uniform3f(gl.getUniformLocation(program, "u_clearColor"), bg[0], bg[1], bg[2]);
  }

  function bindArrowAttributes(engine, scale, translate, resolution, bufferOverride, contrast = 1, clearColor = null) {
    const gl = engine.gl;
    const program = engine._arrowProgram;
    const arrowStride = 9;
    const unitLoc = gl.getAttribLocation(program, "a_unit");
    const posLoc = gl.getAttribLocation(program, "a_pos");
    const dirLoc = gl.getAttribLocation(program, "a_dir");
    const sizeLoc = gl.getAttribLocation(program, "a_size");
    const colorLoc = gl.getAttribLocation(program, "a_color");

    gl.bindBuffer(gl.ARRAY_BUFFER, engine._arrowUnitBuffer);
    gl.enableVertexAttribArray(unitLoc);
    gl.vertexAttribPointer(unitLoc, 2, gl.FLOAT, false, 0, 0);

    gl.bindBuffer(gl.ARRAY_BUFFER, bufferOverride || engine._arrowBuffer);
    const strideBytes = arrowStride * 4;
    gl.enableVertexAttribArray(posLoc);
    gl.vertexAttribPointer(posLoc, 2, gl.FLOAT, false, strideBytes, 0);
    gl.enableVertexAttribArray(dirLoc);
    gl.vertexAttribPointer(dirLoc, 2, gl.FLOAT, false, strideBytes, 8);
    gl.enableVertexAttribArray(sizeLoc);
    gl.vertexAttribPointer(sizeLoc, 1, gl.FLOAT, false, strideBytes, 16);
    gl.enableVertexAttribArray(colorLoc);
    gl.vertexAttribPointer(colorLoc, 4, gl.FLOAT, false, strideBytes, 20);

    if (engine._isWebGL2) {
      gl.vertexAttribDivisor(unitLoc, 0);
      gl.vertexAttribDivisor(posLoc, 1);
      gl.vertexAttribDivisor(dirLoc, 1);
      gl.vertexAttribDivisor(sizeLoc, 1);
      gl.vertexAttribDivisor(colorLoc, 1);
    } else if (engine._instancedExt) {
      engine._instancedExt.vertexAttribDivisorANGLE(unitLoc, 0);
      engine._instancedExt.vertexAttribDivisorANGLE(posLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(dirLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(sizeLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(colorLoc, 1);
    }

    gl.uniform2f(gl.getUniformLocation(program, "u_resolution"), resolution[0], resolution[1]);
    gl.uniform1f(gl.getUniformLocation(program, "u_scale"), scale);
    gl.uniform2f(gl.getUniformLocation(program, "u_translate"), translate.x, translate.y);
    const bg = Array.isArray(clearColor) ? clearColor : engine._clearColor || WEBGL_CLEAR_FALLBACK;
    gl.uniform1f(gl.getUniformLocation(program, "u_contrast"), clamp(Number(contrast) || 1, 0, 1));
    gl.uniform3f(gl.getUniformLocation(program, "u_clearColor"), bg[0], bg[1], bg[2]);
  }

  function bindTextAttributes(engine, scale, translate, resolution, batch, contrast = 1, clearColor = null, alphaScale = 1) {
    const gl = engine.gl;
    const program = engine._textProgram;
    if (!gl || !program || !batch) return;
    const strideBytes = engine._textStride * 4;
    const unitLoc = gl.getAttribLocation(program, "a_unit");
    const posLoc = gl.getAttribLocation(program, "a_pos");
    const sizeLoc = gl.getAttribLocation(program, "a_size");
    const uvLoc = gl.getAttribLocation(program, "a_uv");
    const colorLoc = gl.getAttribLocation(program, "a_color");
    gl.bindBuffer(gl.ARRAY_BUFFER, engine._textUnitBuffer);
    gl.enableVertexAttribArray(unitLoc);
    gl.vertexAttribPointer(unitLoc, 2, gl.FLOAT, false, 0, 0);
    gl.bindBuffer(gl.ARRAY_BUFFER, batch.buffer);
    gl.enableVertexAttribArray(posLoc);
    gl.vertexAttribPointer(posLoc, 2, gl.FLOAT, false, strideBytes, 0);
    gl.enableVertexAttribArray(sizeLoc);
    gl.vertexAttribPointer(sizeLoc, 2, gl.FLOAT, false, strideBytes, 8);
    gl.enableVertexAttribArray(uvLoc);
    gl.vertexAttribPointer(uvLoc, 4, gl.FLOAT, false, strideBytes, 16);
    gl.enableVertexAttribArray(colorLoc);
    gl.vertexAttribPointer(colorLoc, 4, gl.FLOAT, false, strideBytes, 32);

    if (engine._isWebGL2) {
      gl.vertexAttribDivisor(unitLoc, 0);
      gl.vertexAttribDivisor(posLoc, 1);
      gl.vertexAttribDivisor(sizeLoc, 1);
      gl.vertexAttribDivisor(uvLoc, 1);
      gl.vertexAttribDivisor(colorLoc, 1);
    } else if (engine._instancedExt) {
      engine._instancedExt.vertexAttribDivisorANGLE(unitLoc, 0);
      engine._instancedExt.vertexAttribDivisorANGLE(posLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(sizeLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(uvLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(colorLoc, 1);
    }

    gl.uniform2f(gl.getUniformLocation(program, "u_resolution"), resolution[0], resolution[1]);
    gl.uniform1f(gl.getUniformLocation(program, "u_scale"), scale);
    gl.uniform2f(gl.getUniformLocation(program, "u_translate"), translate.x, translate.y);
    const texLoc = gl.getUniformLocation(program, "u_tex");
    gl.uniform1i(texLoc, 0);
    const bg = Array.isArray(clearColor) ? clearColor : engine._clearColor || WEBGL_CLEAR_FALLBACK;
    gl.uniform1f(gl.getUniformLocation(program, "u_contrast"), clamp(Number(contrast) || 1, 0, 1));
    gl.uniform3f(gl.getUniformLocation(program, "u_clearColor"), bg[0], bg[1], bg[2]);
    gl.uniform1f(gl.getUniformLocation(program, "u_alphaScale"), clamp(Number(alphaScale) || 1, 0, 1));
  }

  function bindPickNodeAttributes(engine, scale, translate, resolution) {
    const gl = engine.gl;
    const program = engine._pickNodeProgram;
    if (!gl || !program) return;
    const strideBytes = 7 * 4;
    const unitLoc = gl.getAttribLocation(program, "a_unit");
    const centerLoc = gl.getAttribLocation(program, "a_center");
    const radiusLoc = gl.getAttribLocation(program, "a_radius");
    const pickLoc = gl.getAttribLocation(program, "a_pick");
    gl.bindBuffer(gl.ARRAY_BUFFER, engine._nodeUnitBuffer);
    gl.enableVertexAttribArray(unitLoc);
    gl.vertexAttribPointer(unitLoc, 2, gl.FLOAT, false, 0, 0);
    gl.bindBuffer(gl.ARRAY_BUFFER, engine._pickNodeBuffer);
    gl.enableVertexAttribArray(centerLoc);
    gl.vertexAttribPointer(centerLoc, 2, gl.FLOAT, false, strideBytes, 0);
    gl.enableVertexAttribArray(radiusLoc);
    gl.vertexAttribPointer(radiusLoc, 1, gl.FLOAT, false, strideBytes, 8);
    gl.enableVertexAttribArray(pickLoc);
    gl.vertexAttribPointer(pickLoc, 4, gl.FLOAT, false, strideBytes, 12);
    if (engine._isWebGL2) {
      gl.vertexAttribDivisor(unitLoc, 0);
      gl.vertexAttribDivisor(centerLoc, 1);
      gl.vertexAttribDivisor(radiusLoc, 1);
      gl.vertexAttribDivisor(pickLoc, 1);
    } else if (engine._instancedExt) {
      engine._instancedExt.vertexAttribDivisorANGLE(unitLoc, 0);
      engine._instancedExt.vertexAttribDivisorANGLE(centerLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(radiusLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(pickLoc, 1);
    }
    gl.uniform2f(gl.getUniformLocation(program, "u_resolution"), resolution[0], resolution[1]);
    gl.uniform1f(gl.getUniformLocation(program, "u_scale"), scale);
    gl.uniform2f(gl.getUniformLocation(program, "u_translate"), translate.x, translate.y);
  }

  function bindPickEdgeAttributes(engine, scale, translate, resolution) {
    const gl = engine.gl;
    const program = engine._pickEdgeProgram;
    if (!gl || !program) return;
    const strideBytes = 9 * 4;
    const unitLoc = gl.getAttribLocation(program, "a_unit");
    const startLoc = gl.getAttribLocation(program, "a_start");
    const endLoc = gl.getAttribLocation(program, "a_end");
    const widthLoc = gl.getAttribLocation(program, "a_width");
    const pickLoc = gl.getAttribLocation(program, "a_pick");
    gl.bindBuffer(gl.ARRAY_BUFFER, engine._edgeUnitBuffer);
    gl.enableVertexAttribArray(unitLoc);
    gl.vertexAttribPointer(unitLoc, 2, gl.FLOAT, false, 0, 0);
    gl.bindBuffer(gl.ARRAY_BUFFER, engine._pickEdgeBuffer);
    gl.enableVertexAttribArray(startLoc);
    gl.vertexAttribPointer(startLoc, 2, gl.FLOAT, false, strideBytes, 0);
    gl.enableVertexAttribArray(endLoc);
    gl.vertexAttribPointer(endLoc, 2, gl.FLOAT, false, strideBytes, 8);
    gl.enableVertexAttribArray(widthLoc);
    gl.vertexAttribPointer(widthLoc, 1, gl.FLOAT, false, strideBytes, 16);
    gl.enableVertexAttribArray(pickLoc);
    gl.vertexAttribPointer(pickLoc, 4, gl.FLOAT, false, strideBytes, 20);
    if (engine._isWebGL2) {
      gl.vertexAttribDivisor(unitLoc, 0);
      gl.vertexAttribDivisor(startLoc, 1);
      gl.vertexAttribDivisor(endLoc, 1);
      gl.vertexAttribDivisor(widthLoc, 1);
      gl.vertexAttribDivisor(pickLoc, 1);
    } else if (engine._instancedExt) {
      engine._instancedExt.vertexAttribDivisorANGLE(unitLoc, 0);
      engine._instancedExt.vertexAttribDivisorANGLE(startLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(endLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(widthLoc, 1);
      engine._instancedExt.vertexAttribDivisorANGLE(pickLoc, 1);
    }
    gl.uniform2f(gl.getUniformLocation(program, "u_resolution"), resolution[0], resolution[1]);
    gl.uniform1f(gl.getUniformLocation(program, "u_scale"), scale);
    gl.uniform2f(gl.getUniformLocation(program, "u_translate"), translate.x, translate.y);
  }

  window.__ANALYTIX_GRAPH_ATTRIBUTE_BINDING__ = {
    bindEdgeAttributes,
    bindNodeAttributes,
    bindArrowAttributes,
    bindTextAttributes,
    bindPickNodeAttributes,
    bindPickEdgeAttributes,
  };
})();
