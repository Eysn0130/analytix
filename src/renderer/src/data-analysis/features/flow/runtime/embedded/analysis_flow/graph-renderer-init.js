/* global window */
(() => {
  function initGraphRendererGl(engine, options = {}) {
    if (!engine?.canvas) return;
    const createGraphGlContext = options.createGraphGlContext;
    const WEBGL_CLEAR_FALLBACK = options.WEBGL_CLEAR_FALLBACK || [246 / 255, 247 / 255, 251 / 255, 1];
    const EDGE_AA_V2_ENABLED = options.EDGE_AA_V2_ENABLED !== false;
    const EDGE_MIN_SCREEN_WIDTH_PX = Number(options.EDGE_MIN_SCREEN_WIDTH_PX) || 1.5;
    const EDGE_DASH_LOD_PERIOD_PX = Number(options.EDGE_DASH_LOD_PERIOD_PX) || 4.0;
    const EDGE_DASH_LOD_BLEND_PX = Number(options.EDGE_DASH_LOD_BLEND_PX) || 1.1;
    const EDGE_AA_MIN_PX = Number(options.EDGE_AA_MIN_PX) || 1.2;
    const NODE_AA_PAD_PX = Number(options.NODE_AA_PAD_PX) || 2.25;
    if (typeof createGraphGlContext !== "function") {
      throw new Error("createGraphGlContext missing for renderer init");
    }
    const clearColor = engine._clearColor || WEBGL_CLEAR_FALLBACK;
    const bootstrap = createGraphGlContext(engine.canvas, clearColor);
    const gl = bootstrap.gl;
    engine.gl = gl;
    engine._isWebGL2 = !!bootstrap.isWebGL2;
    engine._instancedExt = bootstrap.instancedExt || null;
    if (!gl) {
      engine.renderer = "webgl";
      engine._useGpuText = true;
      engine._textMode = "gpu";
      engine._useGpuPick = false;
      throw new Error("WebGL context unavailable");
    }
    engine.renderer = "webgl";
    engine._fragPrecision = bootstrap.fragPrecision || "mediump";
    const extDerivatives = gl.getExtension("OES_standard_derivatives");
    let useDerivatives = !!extDerivatives || engine._isWebGL2;
    const buildPrograms = (withDerivatives) => {
      const derivativePrefix =
        !engine._isWebGL2 && withDerivatives ? "#extension GL_OES_standard_derivatives : enable\n" : "";
      const fragPrecision = engine._fragPrecision || "mediump";
      engine._programInfo = {};
      engine._shaderInfo = {};

      const edgeVertV1 =
        `attribute vec2 a_unit;\nattribute vec2 a_start;\nattribute vec2 a_end;\nattribute float a_width;\nattribute vec4 a_color;\nattribute float a_dashType;\nattribute float a_dashSize;\nattribute float a_dashGap;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvoid main(){\n  vec2 dir = a_end - a_start;\n  float len = length(dir);\n  if(len < 0.0001){ dir = vec2(1.0,0.0); len = 1.0; }\n  vec2 dirN = dir / len;\n  vec2 perp = vec2(-dirN.y, dirN.x);\n  float safeScale = max(abs(u_scale), 0.0001);\n  float halfWidthPx = max(0.5 * a_width * safeScale, 0.0001);\n  float aaPad = clamp(1.05 / halfWidthPx, 0.0, 1.35);\n  float edgeExtent = 1.0 + aaPad;\n  vec2 world = a_start + dirN * (a_unit.x * len) + perp * (a_unit.y * edgeExtent * 0.5 * a_width);\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_color = a_color;\n  v_dashType = a_dashType;\n  v_dashSize = a_dashSize;\n  v_dashGap = a_dashGap;\n  v_t = a_unit.x * len;\n  v_edgeY = a_unit.y * edgeExtent;\n}\n`;
      const edgeFragV1 = withDerivatives
        ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  if(v_dashType > 0.5){\n    float period = v_dashSize + v_dashGap;\n    float pos = mod(v_t, period);\n    if(pos > v_dashSize) discard;\n  }\n  float edge = abs(v_edgeY);\n  if(edge > 1.0) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, v_color.a);\n}\n`
        : `precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  if(v_dashType > 0.5){\n    float period = v_dashSize + v_dashGap;\n    float pos = mod(v_t, period);\n    if(pos > v_dashSize) discard;\n  }\n  float edge = abs(v_edgeY);\n  if(edge > 1.0) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, v_color.a);\n}\n`;
      const edgeVertV2 =
        `attribute vec2 a_unit;\nattribute vec2 a_start;\nattribute vec2 a_end;\nattribute float a_width;\nattribute vec4 a_color;\nattribute float a_dashType;\nattribute float a_dashSize;\nattribute float a_dashGap;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvarying float v_halfWidthPx;\nvarying float v_scaleAbs;\nvoid main(){\n  vec2 dir = a_end - a_start;\n  float len = length(dir);\n  if(len < 0.0001){ dir = vec2(1.0,0.0); len = 1.0; }\n  vec2 dirN = dir / len;\n  vec2 perp = vec2(-dirN.y, dirN.x);\n  float safeScale = max(abs(u_scale), 0.0001);\n  float rawHalfWidthPx = max(0.5 * a_width * safeScale, 0.0001);\n  float clampedHalfWidthPx = max(rawHalfWidthPx, ${EDGE_MIN_SCREEN_WIDTH_PX.toFixed(4)} * 0.5);\n  float aaPad = clamp(${EDGE_AA_MIN_PX.toFixed(4)} / clampedHalfWidthPx, 0.0, 1.35);\n  float edgeExtent = 1.0 + aaPad;\n  float halfWorld = clampedHalfWidthPx / safeScale;\n  vec2 world = a_start + dirN * (a_unit.x * len) + perp * (a_unit.y * edgeExtent * halfWorld);\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_color = a_color;\n  v_dashType = a_dashType;\n  v_dashSize = a_dashSize;\n  v_dashGap = a_dashGap;\n  v_t = a_unit.x * len;\n  v_edgeY = a_unit.y * edgeExtent;\n  v_halfWidthPx = clampedHalfWidthPx;\n  v_scaleAbs = safeScale;\n}\n`;
      const edgeFragV2 = withDerivatives
        ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvarying float v_halfWidthPx;\nvarying float v_scaleAbs;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  float dashMaskRaw = 1.0;\n  float period = max(v_dashSize + v_dashGap, 0.0001);\n  if(v_dashType > 0.5){\n    float pos = mod(max(v_t, 0.0), period);\n    dashMaskRaw = step(pos, v_dashSize);\n  }\n  float periodPx = period * v_scaleAbs;\n  float lod = smoothstep(${(EDGE_DASH_LOD_PERIOD_PX - EDGE_DASH_LOD_BLEND_PX).toFixed(4)}, ${(EDGE_DASH_LOD_PERIOD_PX + EDGE_DASH_LOD_BLEND_PX).toFixed(4)}, periodPx);\n  float dashMask = mix(1.0, dashMaskRaw, lod);\n  float edge = abs(v_edgeY);\n  float aa = max(fwidth(v_edgeY), ${EDGE_AA_MIN_PX.toFixed(4)} / max(v_halfWidthPx, 0.0001));\n  float edgeMask = 1.0 - smoothstep(1.0 - aa, 1.0 + aa, edge);\n  float alpha = v_color.a * edgeMask * dashMask;\n  if(alpha <= 0.001) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, alpha);\n}\n`
        : `precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvarying float v_t;\nvarying float v_edgeY;\nvarying float v_halfWidthPx;\nvarying float v_scaleAbs;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  float dashMaskRaw = 1.0;\n  float period = max(v_dashSize + v_dashGap, 0.0001);\n  if(v_dashType > 0.5){\n    float pos = mod(max(v_t, 0.0), period);\n    dashMaskRaw = step(pos, v_dashSize);\n  }\n  float periodPx = period * v_scaleAbs;\n  float lod = smoothstep(${(EDGE_DASH_LOD_PERIOD_PX - EDGE_DASH_LOD_BLEND_PX).toFixed(4)}, ${(EDGE_DASH_LOD_PERIOD_PX + EDGE_DASH_LOD_BLEND_PX).toFixed(4)}, periodPx);\n  float dashMask = mix(1.0, dashMaskRaw, lod);\n  float edge = abs(v_edgeY);\n  float aa = clamp(${EDGE_AA_MIN_PX.toFixed(4)} / max(v_halfWidthPx, 0.0001), 0.02, 0.7);\n  float edgeMask = 1.0 - smoothstep(1.0 - aa, 1.0 + aa, edge);\n  float alpha = v_color.a * edgeMask * dashMask;\n  if(alpha <= 0.001) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, alpha);\n}\n`;
      const edgeVert = EDGE_AA_V2_ENABLED ? edgeVertV2 : edgeVertV1;
      const edgeFrag = EDGE_AA_V2_ENABLED ? edgeFragV2 : edgeFragV1;

      engine._edgeProgram = engine._createProgram(edgeVert, edgeFrag, "edge");

      const nodeFrag = withDerivatives
        ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec2 v_unit;\nvarying float v_radius;\nvarying float v_lineWidth;\nvarying vec4 v_stroke;\nvarying vec4 v_fill;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvoid main(){\n  float dist = length(v_unit);\n  float inner = 1.0 - (v_lineWidth / max(v_radius, 0.0001));\n  inner = clamp(inner, 0.0, 1.0);\n  float aa = max(fwidth(dist), 0.01);\n  float outerMask = 1.0 - smoothstep(1.0 - aa, 1.0 + aa, dist);\n  float innerMask = 1.0 - smoothstep(inner - aa, inner + aa, dist);\n  float strokeMask = clamp(outerMask - innerMask, 0.0, 1.0);\n  float fillMask = innerMask;\n  if(v_dashType > 0.5){\n    float angle = atan(v_unit.y, v_unit.x) + 3.14159265;\n    float pos = mod(angle * max(v_radius, 0.0001), max(v_dashSize + v_dashGap, 0.0001));\n    strokeMask *= step(pos, v_dashSize);\n  }\n  vec3 rgbBase = v_fill.rgb * fillMask + v_stroke.rgb * strokeMask;\n  float alpha = v_fill.a * fillMask + v_stroke.a * strokeMask;\n  if(alpha <= 0.01) discard;\n  float contrast = max(u_contrast, 0.0);\n  vec3 rgb = clamp(u_clearColor + (rgbBase - u_clearColor) * contrast, 0.0, 1.0);\n  gl_FragColor = vec4(rgb, alpha);\n}\n`
        : `precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec2 v_unit;\nvarying float v_radius;\nvarying float v_lineWidth;\nvarying vec4 v_stroke;\nvarying vec4 v_fill;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvoid main(){\n  float dist = length(v_unit);\n  float inner = 1.0 - (v_lineWidth / max(v_radius, 0.0001));\n  inner = clamp(inner, 0.0, 1.0);\n  float outerMask = 1.0 - smoothstep(0.97, 1.03, dist);\n  float innerMask = 1.0 - smoothstep(inner - 0.03, inner + 0.03, dist);\n  float strokeMask = clamp(outerMask - innerMask, 0.0, 1.0);\n  float fillMask = innerMask;\n  if(v_dashType > 0.5){\n    float angle = atan(v_unit.y, v_unit.x) + 3.14159265;\n    float pos = mod(angle * max(v_radius, 0.0001), max(v_dashSize + v_dashGap, 0.0001));\n    strokeMask *= step(pos, v_dashSize);\n  }\n  vec3 rgbBase = v_fill.rgb * fillMask + v_stroke.rgb * strokeMask;\n  float alpha = v_fill.a * fillMask + v_stroke.a * strokeMask;\n  if(alpha <= 0.01) discard;\n  float contrast = max(u_contrast, 0.0);\n  vec3 rgb = clamp(u_clearColor + (rgbBase - u_clearColor) * contrast, 0.0, 1.0);\n  gl_FragColor = vec4(rgb, alpha);\n}\n`;

      engine._nodeProgram = engine._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_center;\nattribute float a_radius;\nattribute float a_lineWidth;\nattribute vec4 a_stroke;\nattribute vec4 a_fill;\nattribute float a_dashType;\nattribute float a_dashSize;\nattribute float a_dashGap;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec2 v_unit;\nvarying float v_radius;\nvarying float v_lineWidth;\nvarying vec4 v_stroke;\nvarying vec4 v_fill;\nvarying float v_dashType;\nvarying float v_dashSize;\nvarying float v_dashGap;\nvoid main(){\n  float safeRadius = max(a_radius, 0.0001);\n  float safeScale = max(abs(u_scale), 0.0001);\n  float aaPadWorld = ${NODE_AA_PAD_PX.toFixed(4)} / safeScale;\n  float outerRadius = safeRadius + aaPadWorld;\n  vec2 paddedUnit = a_unit * (outerRadius / safeRadius);\n  vec2 world = a_center + a_unit * outerRadius;\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_unit = paddedUnit;\n  v_radius = safeRadius;\n  v_lineWidth = a_lineWidth;\n  v_stroke = a_stroke;\n  v_fill = a_fill;\n  v_dashType = a_dashType;\n  v_dashSize = a_dashSize;\n  v_dashGap = a_dashGap;\n}\n`,
        nodeFrag,
        "node"
      );

      const arrowFrag = withDerivatives
        ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying vec2 v_unit;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, v_color.a);\n}\n`
        : `precision ${fragPrecision} float;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nvarying vec4 v_color;\nvarying vec2 v_unit;\nvoid main(){\n  if(v_color.a <= 0.0) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, v_color.a);\n}\n`;

      engine._arrowProgram = engine._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_pos;\nattribute vec2 a_dir;\nattribute float a_size;\nattribute vec4 a_color;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec4 v_color;\nvarying vec2 v_unit;\nvoid main(){\n  vec2 perp = vec2(-a_dir.y, a_dir.x);\n  vec2 world = a_pos + a_dir * (a_unit.x * a_size) + perp * (a_unit.y * a_size);\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_color = a_color;\n  v_unit = a_unit;\n}\n`,
        arrowFrag,
        "arrow"
      );

      const textFrag = withDerivatives
        ? `${derivativePrefix}precision ${fragPrecision} float;\nuniform sampler2D u_tex;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nuniform float u_alphaScale;\nvarying vec2 v_uv;\nvarying vec4 v_color;\nvoid main(){\n  float a = texture2D(u_tex, v_uv).a;\n  float outAlpha = v_color.a * a * clamp(u_alphaScale, 0.0, 1.0);\n  if(outAlpha <= 0.01) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, outAlpha);\n}\n`
        : `precision ${fragPrecision} float;\nuniform sampler2D u_tex;\nuniform float u_contrast;\nuniform vec3 u_clearColor;\nuniform float u_alphaScale;\nvarying vec2 v_uv;\nvarying vec4 v_color;\nvoid main(){\n  float a = texture2D(u_tex, v_uv).a;\n  float outAlpha = v_color.a * a * clamp(u_alphaScale, 0.0, 1.0);\n  if(outAlpha <= 0.01) discard;\n  float contrast = clamp(u_contrast, 0.0, 1.0);\n  vec3 rgb = mix(u_clearColor, v_color.rgb, contrast);\n  gl_FragColor = vec4(rgb, outAlpha);\n}\n`;
      engine._textProgram = engine._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_pos;\nattribute vec2 a_size;\nattribute vec4 a_uv;\nattribute vec4 a_color;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec2 v_uv;\nvarying vec4 v_color;\nvoid main(){\n  vec2 world = a_pos + a_unit * a_size;\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  vec2 unit = a_unit + vec2(0.5, 0.5);\n  v_uv = mix(a_uv.xy, a_uv.zw, unit);\n  v_color = a_color;\n}\n`,
        textFrag,
        "text"
      );

      engine._pickNodeProgram = engine._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_center;\nattribute float a_radius;\nattribute vec4 a_pick;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec2 v_unit;\nvarying vec4 v_pick;\nvoid main(){\n  vec2 world = a_center + a_unit * a_radius;\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_unit = a_unit;\n  v_pick = a_pick;\n}\n`,
        `precision mediump float;\nvarying vec2 v_unit;\nvarying vec4 v_pick;\nvoid main(){\n  float dist = length(v_unit);\n  if(dist > 1.0) discard;\n  gl_FragColor = v_pick;\n}\n`,
        "pickNode"
      );

      engine._pickEdgeProgram = engine._createProgram(
        `attribute vec2 a_unit;\nattribute vec2 a_start;\nattribute vec2 a_end;\nattribute float a_width;\nattribute vec4 a_pick;\nuniform vec2 u_resolution;\nuniform float u_scale;\nuniform vec2 u_translate;\nvarying vec4 v_pick;\nvoid main(){\n  vec2 dir = a_end - a_start;\n  float len = length(dir);\n  if(len < 0.0001){ dir = vec2(1.0,0.0); len = 1.0; }\n  vec2 dirN = dir / len;\n  vec2 perp = vec2(-dirN.y, dirN.x);\n  vec2 world = a_start + dirN * (a_unit.x * len) + perp * (a_unit.y * 0.5 * a_width);\n  vec2 screen = world * u_scale + u_translate;\n  vec2 clip = screen / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);\n  v_pick = a_pick;\n}\n`,
        `precision mediump float;\nvarying vec4 v_pick;\nvoid main(){\n  gl_FragColor = v_pick;\n}\n`,
        "pickEdge"
      );
    };

    buildPrograms(useDerivatives);
    if (useDerivatives) {
      const failed = Object.values(engine._shaderInfo || {}).some((info) => !info.compiled && /fwidth|deriv/i.test(info.log || ""));
      if (failed) {
        engine._emitWebglWarn("derivatives-fallback", "WebGL derivatives unavailable, fallback to basic shaders");
        useDerivatives = false;
        engine._releaseGlPrograms();
        buildPrograms(false);
      }
    }
    engine._hasDerivatives = useDerivatives;
    if (engine._programInfo?.text && engine._programInfo.text.linked === false) {
      engine._useGpuText = false;
      engine._textMode = "gpu";
    }

    engine._initBuffers();
  }

  window.__ANALYTIX_GRAPH_RENDERER_INIT__ = {
    initGraphRendererGl,
  };
})();
