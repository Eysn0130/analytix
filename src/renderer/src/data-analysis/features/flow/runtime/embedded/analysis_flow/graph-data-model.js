(function () {
  const root = typeof window !== "undefined" ? window : globalThis;
  const UNKNOWN_ACCOUNT_LABELS = ["未知卡号", "账号未知", "未知账号"];
  const PLACEHOLDER_TOKEN_PREFIX = "__cp_placeholder__::";
  const PLACEHOLDER_KIND_LABELS = {
    db_null: "NULL",
    empty: "空串",
    slash_n: "\\N",
    dash: "-",
    emdash: "—",
    fw_dash: "－",
    literal_null: "null",
    literal_none: "none",
    literal_nan: "nan",
  };
  const GRAPH_THEME_PALETTES = Object.freeze({
    light: Object.freeze({
      nodeStroke: "rgba(2,6,23,.72)",
      edgeStroke: "rgba(2,6,23,.68)",
      nodeFill: "rgba(255,255,255,0.05)",
      nodeText: "rgba(2,6,23,.86)",
      nodeSubText: "rgba(2,6,23,.72)",
      edgeText: "rgba(2,6,23,.82)",
    }),
    dark: Object.freeze({
      nodeStroke: "rgba(148,163,184,.78)",
      edgeStroke: "rgba(148,163,184,.72)",
      nodeFill: "rgba(148,163,184,.14)",
      nodeText: "rgba(236,242,255,.94)",
      nodeSubText: "rgba(203,213,225,.78)",
      edgeText: "rgba(236,242,255,.9)",
    }),
  });
  const NODE_DISPLAY_RESERVED_FIELDS = new Set([
    "id",
    "title",
    "label",
    "name",
    "display_id",
    "displayId",
    "display_ids",
    "displayIds",
    "displayIdRaw",
    "display_id_raw",
    "mergedDisplay",
    "type",
    "r",
    "stroke",
    "fill",
    "lineWidth",
    "line_width",
    "nodeShadow",
    "node_shadow",
    "icon",
    "iconSymbol",
    "icon_symbol",
    "iconSize",
    "icon_size",
    "nodeShape",
    "node_shape",
    "nodeDash",
    "node_dash",
    "fontFamily",
    "font_family",
    "fontSize",
    "font_size",
    "fontBold",
    "font_bold",
    "fontItalic",
    "font_italic",
    "fontUnderline",
    "font_underline",
    "fontShadow",
    "font_shadow",
    "ntype",
    "total_amount",
    "total_count",
    "nodeRenderMode",
    "node_render_mode",
    "nodeLabelHidden",
    "node_label_hidden",
    "layoutVisibilityTier",
    "layout_visibility_tier",
    "layoutLabelTier",
    "layout_label_tier",
    "layoutHiddenBySkeleton",
    "layout_hidden_by_skeleton",
    "layoutCommunity",
    "layout_community",
    "layoutBridgeScore",
    "layout_bridge_score",
    "layoutStable",
    "layout_stable",
    "cluster_node",
    "cluster_id",
    "cluster_anchor_id",
    "cluster_member_count",
    "projection_cluster_id",
    "projection_visible",
    "projection_collapsed",
  ]);
  const EDGE_DISPLAY_RESERVED_FIELDS = new Set([
    "id",
    "source",
    "target",
    "type",
    "mode",
    "label",
    "labelTop",
    "label_top",
    "labelBottom",
    "label_bottom",
    "detailLabel",
    "detail_label",
    "edgeLabelHidden",
    "edge_label_hidden",
    "userCreated",
    "user_created",
    "stroke",
    "lineWidth",
    "line_width",
    "edgeDash",
    "edge_dash",
    "edgeArrow",
    "edge_arrow",
    "arrow",
    "showArrow",
    "show_arrow",
    "gap",
    "holePad",
    "hole_pad",
    "outPad",
    "out_pad",
    "textColor",
    "text_color",
    "fontFamily",
    "font_family",
    "fontSize",
    "font_size",
    "fontBold",
    "font_bold",
    "fontItalic",
    "font_italic",
    "fontUnderline",
    "font_underline",
    "fontShadow",
    "font_shadow",
    "amount",
    "count",
    "in_amount",
    "out_amount",
    "forward_amount",
    "reverse_amount",
    "first_time",
    "last_time",
    "projection_edge",
  ]);

  function resolveGraphThemeName(options = {}) {
    const explicit = String(options?.theme || options?.themeName || options?.colorScheme || options?.style?.theme || "")
      .trim()
      .toLowerCase();
    if (explicit.includes("dark")) return "dark";
    if (explicit.includes("light")) return "light";
    try {
      const doc = typeof document !== "undefined" ? document : null;
      const rootEl = doc?.documentElement;
      const attr = String(rootEl?.dataset?.theme || rootEl?.getAttribute?.("data-theme") || "")
        .trim()
        .toLowerCase();
      if (attr.includes("dark")) return "dark";
      if (attr.includes("light")) return "light";
    } catch (e) {}
    return "light";
  }

  function resolveGraphThemePalette(options = {}) {
    return GRAPH_THEME_PALETTES[resolveGraphThemeName(options)] || GRAPH_THEME_PALETTES.light;
  }

  function isUnknownAccountLabel(text, { exact = false } = {}) {
    const value = String(text || "").trim();
    if (!value) return false;
    if (exact) return UNKNOWN_ACCOUNT_LABELS.includes(value);
    return UNKNOWN_ACCOUNT_LABELS.some((label) => value.includes(label));
  }

  function stripDisplayIdPrefix(value) {
    return String(value || "")
      .trim()
      .replace(/^(卡号|账号|账户)\s*[:：]\s*/i, "")
      .trim();
  }

  function parseDisplayIdList(raw) {
    const text = String(raw || "").trim();
    if (!text) return [];
    if (text.includes("账户合集")) return [];
    if (isUnknownAccountLabel(text)) return [];
    const cleaned = text.replace(/\s*等\d+个\s*$/, "");
    return cleaned
      .split(/\s*[/／]\s*/)
      .map((val) => stripDisplayIdPrefix(String(val || "").trim()))
      .filter(Boolean);
  }

  function normalizeDisplayIds(raw) {
    let list = [];
    if (Array.isArray(raw)) list = raw;
    else if (raw instanceof Set) list = Array.from(raw);
    else if (raw != null) list = parseDisplayIdList(raw);
    const out = [];
    const seen = new Set();
    list.forEach((val) => {
      const s = stripDisplayIdPrefix(String(val || "").trim());
      if (!s) return;
      if (isUnknownAccountLabel(s)) return;
      if (seen.has(s)) return;
      seen.add(s);
      out.push(s);
    });
    return out;
  }

  function formatDisplayIdLabel(list, fallback = "", opts = {}) {
    const allowGroup = !!opts.allowGroup;
    const fallbackText = String(fallback || "").trim();
    if (Array.isArray(list) && list.length) {
      if (allowGroup && list.length > 1) return `账户合集（${list.length}）`;
      if (list.length === 1) return list[0];
      if (fallbackText && !fallbackText.includes("账户合集")) return fallbackText;
      return list.join(" / ");
    }
    if (!allowGroup && fallbackText.includes("账户合集")) return "";
    return fallbackText || "";
  }

  function coerceBool(value, fallback = false) {
    if (value == null) return !!fallback;
    if (typeof value === "boolean") return value;
    if (typeof value === "number") return Number.isFinite(value) && value !== 0;
    if (typeof value === "string") {
      const text = value.trim().toLowerCase();
      if (!text) return false;
      if (["0", "false", "no", "off", "null", "undefined", "nan"].includes(text)) return false;
      if (["1", "true", "yes", "on"].includes(text)) return true;
      return true;
    }
    return !!value;
  }

  function formatMoneyAmount(value) {
    const num = Number(value) || 0;
    return num.toLocaleString("zh-CN", { maximumFractionDigits: 2, minimumFractionDigits: 0 });
  }

  function normalizeEdgeArrowValue(value, fallback = "end") {
    return ["none", "end", "start", "both"].includes(value) ? value : fallback;
  }

  function cloneGraphExtraValue(value) {
    if (value == null) return null;
    if (["string", "number", "boolean"].includes(typeof value)) return value;
    if (Array.isArray(value)) {
      return value
        .map((item) => (["string", "number", "boolean"].includes(typeof item) ? item : null))
        .filter((item) => item != null);
    }
    return null;
  }

  function collectGraphExtraFields(row, reservedFields) {
    const out = {};
    if (!row || typeof row !== "object" || Array.isArray(row)) return out;
    Object.keys(row).forEach((key) => {
      if (!key || reservedFields.has(key) || key.startsWith("__")) return;
      const cloned = cloneGraphExtraValue(row[key]);
      if (cloned == null) return;
      if (Array.isArray(cloned) && !cloned.length) return;
      out[key] = cloned;
    });
    return out;
  }

  function normalizeKeyValue(value) {
    const raw = String(value || "").trim();
    if (!raw) return "";
    let out = raw.replace(/\s+/g, "");
    const dash = out.indexOf("-");
    const under = out.indexOf("_");
    if (dash > 0 || under > 0) {
      const idx = dash > 0 && under > 0 ? Math.min(dash, under) : dash > 0 ? dash : under;
      out = out.slice(0, idx);
    }
    return out;
  }

  function normalizePlaceholderKind(value) {
    const text = String(value || "").trim();
    return Object.prototype.hasOwnProperty.call(PLACEHOLDER_KIND_LABELS, text) ? text : "";
  }

  function placeholderKindFromToken(value) {
    const text = String(value || "").trim();
    if (!text.startsWith(PLACEHOLDER_TOKEN_PREFIX)) return "";
    const remainder = text.slice(PLACEHOLDER_TOKEN_PREFIX.length).split("::name::", 1)[0];
    return normalizePlaceholderKind(remainder);
  }

  function collectNodeDisplayIds(node) {
    if (!node) return [];
    const picks = [];
    const pushVal = (val) => {
      if (val == null) return;
      if (Array.isArray(val)) val.forEach((item) => item != null && picks.push(item));
      else picks.push(val);
    };
    pushVal(node.displayIds);
    pushVal(node.display_ids);
    pushVal(node.displayIdRaw);
    pushVal(node.display_id_raw);
    pushVal(node.display_id);
    pushVal(node.displayId);

    const out = [];
    const seen = new Set();
    picks.forEach((val) => {
      const parsed = normalizeDisplayIds(val);
      parsed.forEach((id) => {
        if (seen.has(id)) return;
        seen.add(id);
        out.push(id);
      });
    });
    return out;
  }

  function normalizeNodeDisplay(node, opts = {}) {
    if (!node) return;
    const ids = collectNodeDisplayIds(node);
    if (!ids.length) return;
    const raw = ids.join(" / ");
    node.displayIds = ids;
    node.displayIdRaw = raw;
    node.displayId = formatDisplayIdLabel(ids, raw, opts);
  }

  function applyUnknownNameLabel(node, enabled) {
    if (!enabled || !node) return;
    const name = String(node.name || "").trim();
    if (name) return;
    const displayId = String(
      node.displayId ||
        node.display_id ||
        node.displayIdRaw ||
        node.display_id_raw ||
        ""
    ).trim();
    if (!displayId) return;
    if (displayId.includes("账户合集")) return;
    if (isUnknownAccountLabel(displayId)) return;
    const ids = collectNodeDisplayIds(node);
    if (ids.length > 1) return;
    node.name = "未知户名";
  }

  function resolveFocusIdFromNodes(nodes, focusId, focusName) {
    if (!nodes || !nodes.length) return "";
    const direct = nodes.find((node) => node.id === focusId);
    if (direct) return direct.id;
    const norm = normalizeKeyValue(focusId);
    if (norm) {
      const match = nodes.find((node) => normalizeKeyValue(node.id) === norm);
      if (match) return match.id;
    }
    if (focusName) {
      const match = nodes.find((node) => String(node.title || "").includes(focusName));
      if (match) return match.id;
    }
    return "";
  }

  function normalizeStringList(value) {
    let list = [];
    if (Array.isArray(value)) {
      list = value;
    } else if (value instanceof Set || (value && typeof value.forEach === "function" && typeof value.size === "number")) {
      value.forEach((item) => list.push(item));
    }
    return list.map((item) => String(item || "").trim()).filter(Boolean);
  }

  function resolveGraphFocus(nodes, options = {}) {
    const selectedIds = normalizeStringList(options.selectedIds);
    let focusId = resolveFocusIdFromNodes(nodes, options.focusId, options.focusName);
    if (!focusId && selectedIds.length === 1) {
      const only = selectedIds[0];
      if (nodes.some((node) => node.id === only)) focusId = only;
    }
    if (!focusId) {
      const seed = nodes.find((node) => node.ntype === "seed");
      if (seed) focusId = seed.id;
    }

    const focusName = String(options.focusName || options.focusLabel || "").trim();
    const focusNames = normalizeStringList(options.focusNames);
    const focusPlaceholderKinds = normalizeStringList(options.focusPlaceholderKinds)
      .map((kind) => normalizePlaceholderKind(kind))
      .filter(Boolean);
    const focusIds = new Set();
    const matchFocusName = (needle) => {
      if (!needle) return;
      nodes.forEach((node) => {
        const title = String(node.title || "").trim();
        const name = String(node.name || "").trim();
        const id = String(node.id || "").trim();
        const displayId = String(node.displayId || node.display_id || "").trim();
        if (
          (title && title.includes(needle)) ||
          (name && name.includes(needle)) ||
          (id && id === needle) ||
          (displayId && displayId === needle)
        ) {
          focusIds.add(node.id);
        }
      });
    };

    if (focusName) matchFocusName(focusName);
    if (focusNames.length) focusNames.forEach((name) => matchFocusName(name));
    if (options.focusUnknownName) {
      nodes.forEach((node) => {
        const id = String(node.id || "").trim();
        const title = String(node.title || "").trim();
        const name = String(node.name || "").trim();
        const displayId = String(node.displayId || node.display_id || "").trim();
        if (
          id === "__unknown_cp__name::__empty__" ||
          name === "未知户名" ||
          title === "未知户名" ||
          isUnknownAccountLabel(displayId, { exact: true })
        ) {
          focusIds.add(node.id);
        }
      });
    }
    if (options.includeMissingCounterparty) {
      nodes.forEach((node) => {
        const id = String(node.id || "").trim();
        const title = String(node.title || "").trim();
        const name = String(node.name || "").trim();
        const displayId = String(node.displayId || node.display_id || "").trim();
        if (
          id.startsWith("__unknown_cp__name::") ||
          name === "未知户名" ||
          title === "未知户名" ||
          isUnknownAccountLabel(displayId, { exact: true })
        ) {
          focusIds.add(node.id);
        }
      });
    }
    if (focusPlaceholderKinds.length) {
      nodes.forEach((node) => {
        const id = String(node.id || "").trim();
        if (focusPlaceholderKinds.includes(placeholderKindFromToken(id))) {
          focusIds.add(node.id);
        }
      });
    }
    if (!focusIds.size && focusId) focusIds.add(focusId);
    return { focusId, focusIds };
  }

  function resolveNetMainAmounts(edge, mainId) {
    if (!mainId) return { inToMain: 0, outFromMain: 0 };
    const outAmt = Math.abs(Number(edge?.out_amount) || 0);
    const inAmt = Math.abs(Number(edge?.in_amount) || 0);
    const bigger = Math.max(outAmt, inAmt);
    const smaller = Math.min(outAmt, inAmt);
    if (mainId === edge.source) return { inToMain: smaller, outFromMain: bigger };
    if (mainId === edge.target) return { inToMain: bigger, outFromMain: smaller };
    return { inToMain: 0, outFromMain: 0 };
  }

  function cloneFlowSummaryBoundary(value) {
    const source = value && typeof value === "object" && !Array.isArray(value) ? value : null;
    const codes = {
      unloaded: "no_result_loaded",
      blocked: "publication_blocked",
      unknown: "host_verification_missing",
      partial: "partial_coverage",
    };
    const status = String(source?.status || "").trim();
    if (
      !source ||
      source.contract !== "FlowGraphSummaryV1" ||
      !Object.prototype.hasOwnProperty.call(codes, status) ||
      source.factAnswerAllowed !== false ||
      source.nodes !== null ||
      source.edges !== null ||
      source.amount !== null ||
      source.boundaryCode !== codes[status]
    ) {
      return null;
    }
    return {
      contract: "FlowGraphSummaryV1",
      status,
      factAnswerAllowed: false,
      layoutLabel: String(source.layoutLabel || "布局").trim() || "布局",
      nodes: null,
      edges: null,
      amount: null,
      boundaryCode: codes[status],
    };
  }

  function cloneGraphData(data) {
    const nodes = Array.isArray(data?.nodes) ? data.nodes.map((node) => ({ ...node })) : [];
    const edges = Array.isArray(data?.edges) ? data.edges.map((edge) => ({ ...edge })) : [];
    const out = { nodes, edges };
    const runtimeRevision = Number(data?.runtime_revision);
    if (Number.isFinite(runtimeRevision)) out.runtime_revision = runtimeRevision;
    const graphTier = String(data?.graph_tier || data?.graphTier || "").trim();
    if (graphTier) out.graph_tier = graphTier;
    const renderHints =
      data?.render_hints && typeof data.render_hints === "object" && !Array.isArray(data.render_hints)
        ? { ...data.render_hints }
        : data?.renderHints && typeof data.renderHints === "object" && !Array.isArray(data.renderHints)
        ? { ...data.renderHints }
        : null;
    if (renderHints) out.render_hints = renderHints;
    const projection =
      data?.projection && typeof data.projection === "object" && !Array.isArray(data.projection)
        ? JSON.parse(JSON.stringify(data.projection))
        : null;
    if (projection) out.projection = projection;
    const snapshotRef =
      data?.result_snapshot_ref && typeof data.result_snapshot_ref === "object" && !Array.isArray(data.result_snapshot_ref)
        ? { ...data.result_snapshot_ref }
        : data?.resultSnapshotRef && typeof data.resultSnapshotRef === "object" && !Array.isArray(data.resultSnapshotRef)
        ? { ...data.resultSnapshotRef }
        : null;
    if (snapshotRef) out.result_snapshot_ref = snapshotRef;
    const flowSummary = cloneFlowSummaryBoundary(data?.flow_summary || data?.flowSummary);
    if (flowSummary) out.flow_summary = flowSummary;
    return out;
  }

  function cloneResultSnapshotRef(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return null;
    const snapshotId = String(value.snapshot_id || value.snapshotId || "").trim();
    const graphHash = String(value.graph_hash || value.graphHash || "").trim();
    if (!snapshotId && !graphHash) return null;
    return {
      snapshot_id: snapshotId,
      graph_hash: graphHash,
      node_count: Math.max(0, Number(value.node_count || value.nodeCount) || 0),
      edge_count: Math.max(0, Number(value.edge_count || value.edgeCount) || 0),
      stored_at: String(value.stored_at || value.storedAt || "").trim(),
    };
  }

  function cloneGraphProjection(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return null;
    try {
      const cloned = JSON.parse(JSON.stringify(value));
      if (cloned?.source_result_snapshot_ref) {
        cloned.source_result_snapshot_ref = cloneResultSnapshotRef(cloned.source_result_snapshot_ref);
      }
      return cloned;
    } catch (e) {
      return null;
    }
  }

  function normalizeSnapshotGraphPayload(value) {
    const source = value && typeof value === "object" ? value : {};
    return {
      nodes: Array.isArray(source.nodes) ? source.nodes : [],
      edges: Array.isArray(source.edges) ? source.edges : [],
    };
  }

  function buildGraphSnapshotData(data, metadata = {}) {
    const snapshot = cloneGraphData(data);
    snapshot.graph_tier = String(metadata?.graphTier ?? metadata?.graph_tier ?? "").trim();
    const renderHints = metadata?.renderHints ?? metadata?.render_hints;
    snapshot.render_hints =
      renderHints && typeof renderHints === "object" && !Array.isArray(renderHints) ? { ...renderHints } : undefined;
    snapshot.projection = cloneGraphProjection(metadata?.projection);
    snapshot.result_snapshot_ref = cloneResultSnapshotRef(metadata?.resultSnapshotRef || metadata?.result_snapshot_ref);
    const runtimeRevision = Number(metadata?.runtimeRevision ?? metadata?.runtime_revision);
    snapshot.runtime_revision = Number.isFinite(runtimeRevision) ? runtimeRevision : snapshot.runtime_revision;
    return snapshot;
  }

  function normalizeRawGraphPayload(res) {
    const nodesRaw = Array.isArray(res?.nodes) ? res.nodes : [];
    const edgesRaw = Array.isArray(res?.edges) ? res.edges : [];
    const nodeMap = new Map();
    const nodeRemap = new Map();
    nodesRaw.forEach((node) => {
      const id = String(node?.id || "").trim();
      if (!id) return;
      const title = String(node?.title || node?.label || "").trim();
      const name = String(node?.name || "").trim();
      const displayIdRaw = String(node?.display_id || node?.displayId || node?.id || "").trim();
      const displayIds = normalizeDisplayIds(node?.display_ids || node?.displayIds || displayIdRaw);
      const keyBase = `${name || title || ""}||${displayIdRaw || ""}`.trim();
      const key = keyBase && keyBase !== "||" ? keyBase : id;
      const existing = nodeMap.get(key);
      if (!existing) {
        nodeMap.set(key, { raw: node, id, title, name, displayIdRaw, displayIds });
        return;
      }
      if (existing.id === id) return;
      nodeRemap.set(id, existing.id);
      const existingAmount = Number(existing.raw?.total_amount) || 0;
      const existingCount = Number(existing.raw?.total_count) || 0;
      const nextAmount = Number(node?.total_amount) || 0;
      const nextCount = Number(node?.total_count) || 0;
      existing.displayIds = normalizeDisplayIds([...(existing.displayIds || []), ...(displayIds || [])]);
      existing.raw = {
        ...existing.raw,
        total_amount: existingAmount + nextAmount,
        total_count: existingCount + nextCount,
        ntype: existing.raw?.ntype === "seed" || node?.ntype === "seed" ? "seed" : existing.raw?.ntype,
      };
    });
    const nodeEntries = Array.from(nodeMap.values()).map(({ raw, id, title, name, displayIdRaw, displayIds }) => {
      const displayList = Array.isArray(displayIds) && displayIds.length ? displayIds : normalizeDisplayIds(displayIdRaw);
      const mergedDisplay = !!raw?.mergedDisplay;
      return {
        raw,
        id,
        title,
        name,
        displayIdRaw,
        displayIds: displayList,
        displayId: formatDisplayIdLabel(displayList, displayIdRaw, { allowGroup: mergedDisplay }),
        mergedDisplay,
      };
    });
    const edgeEntries = edgesRaw
      .map((edge) => {
        const sourceRaw = String(edge?.source || "").trim();
        const targetRaw = String(edge?.target || "").trim();
        const source = nodeRemap.get(sourceRaw) || sourceRaw;
        const target = nodeRemap.get(targetRaw) || targetRaw;
        if (!source || !target) return null;
        return {
          raw: edge,
          sourceRaw,
          targetRaw,
          source,
          target,
          isSelfLoop: source === target,
        };
      })
      .filter(Boolean);
    return { nodeEntries, edgeEntries };
  }

  function buildGraphNodeDisplayModel(entry, options = {}) {
    if (!entry || typeof entry !== "object") return null;
    const raw = entry.raw || {};
    const id = String(entry.id || "").trim();
    if (!id) return null;
    const style = options?.style && typeof options.style === "object" ? options.style : {};
    const palette = resolveGraphThemePalette(options);
    const isSeed = !!options?.isSeed;
    const defaultFontFamily = String(options?.defaultFontFamily || "").trim() || "PingFang SC";
    const baseSize = style.nodeSize ?? 18;
    const seedSize = Math.max(baseSize + 4, 22);
    const r = Math.max(14, Math.min(30, Number(raw?.r) || (isSeed ? seedSize : baseSize)));
    const stroke = raw?.stroke || raw?.nodeStroke || raw?.node_stroke || style.nodeColor || (isSeed ? "#1f6feb" : palette.nodeStroke);
    const fill = raw?.fill || raw?.nodeFill || raw?.node_fill || style.nodeFill || palette.nodeFill;
    const iconSymbol = style.iconSymbol || "";
    const icon = iconSymbol ? "" : isSeed ? "★" : "";
    const layoutVisibilityTier = String(raw?.layoutVisibilityTier || raw?.layout_visibility_tier || "").trim();
    const layoutLabelTier = String(raw?.layoutLabelTier || raw?.layout_label_tier || "").trim();
    return {
      ...collectGraphExtraFields(raw, NODE_DISPLAY_RESERVED_FIELDS),
      id,
      title: entry.title || id,
      name: entry.name || "",
      displayId: entry.displayId || "",
      displayIdRaw: entry.displayIdRaw || "",
      displayIds: Array.isArray(entry.displayIds) ? entry.displayIds : [],
      mergedDisplay: !!entry.mergedDisplay,
      type: "hollow-entity",
      r,
      stroke,
      fill,
      lineWidth: Number(raw?.lineWidth ?? raw?.line_width ?? style.nodeWidth) || (isSeed ? 3 : 2),
      textColor: style.textColor || palette.nodeText,
      subTextColor: style.textColor || palette.nodeSubText,
      icon,
      iconSymbol,
      iconSize: style.iconSize || 16,
      nodeShape: style.nodeShape || "circle",
      nodeDash: String(raw?.nodeDash || raw?.node_dash || style.nodeDash || ""),
      fontFamily: style.fontFamily || defaultFontFamily,
      fontSize: style.fontSize || 13,
      fontBold: coerceBool(style.fontBold, false),
      fontItalic: coerceBool(style.fontItalic, false),
      fontUnderline: coerceBool(style.fontUnderline, false),
      fontShadow: coerceBool(style.fontShadow, false),
      nodeShadow: coerceBool(style.nodeShadow, false),
      ntype: raw?.ntype || (isSeed ? "seed" : "node"),
      total_amount: raw?.total_amount ?? null,
      total_count: raw?.total_count ?? null,
      nodeRenderMode: String(raw?.nodeRenderMode || "").trim().toLowerCase() || undefined,
      nodeLabelHidden: !!raw?.nodeLabelHidden || layoutLabelTier === "hidden",
      layoutVisibilityTier,
      layoutLabelTier,
      layoutHiddenBySkeleton: !!raw?.layoutHiddenBySkeleton || layoutVisibilityTier === "hidden",
      layoutCommunity: String(raw?.layoutCommunity || raw?.layout_community || "").trim(),
      layoutBridgeScore: Number(raw?.layoutBridgeScore ?? raw?.layout_bridge_score ?? 0) || 0,
      layoutStable: !!raw?.layoutStable || !!raw?.layout_stable,
      cluster_node: !!raw?.cluster_node,
      cluster_id: String(raw?.cluster_id || "").trim(),
      cluster_anchor_id: String(raw?.cluster_anchor_id || "").trim(),
      cluster_member_count: Math.max(0, Number(raw?.cluster_member_count) || 0),
      projection_cluster_id: String(raw?.projection_cluster_id || "").trim(),
      projection_visible: raw?.projection_visible !== false,
      projection_collapsed: !!raw?.projection_collapsed,
    };
  }

  function buildGraphEdgeDisplayModel(entry, options = {}) {
    if (!entry || typeof entry !== "object") return null;
    const raw = entry.raw || {};
    const source = String(entry.source || "").trim();
    const target = String(entry.target || "").trim();
    if (!source || !target) return null;
    const style = options?.style && typeof options.style === "object" ? options.style : {};
    const palette = resolveGraphThemePalette(options);
    const defaultFontFamily = String(options?.defaultFontFamily || "").trim() || "PingFang SC";
    const amount = Number(raw?.amount) || 0;
    const count = Number(raw?.count) || 0;
    const inAmt = Number(raw?.in_amount) || 0;
    const outAmt = Number(raw?.out_amount) || 0;
    const mode = raw?.mode || (inAmt > 0 && outAmt > 0 ? "double" : "single");
    const label = raw?.label || (amount ? `￥${formatMoneyAmount(amount)}` : count ? `${count}` : "");
    const forwardAmt = Number(raw?.forward_amount ?? 0) || 0;
    const reverseAmt = Number(raw?.reverse_amount ?? 0) || 0;
    const labelTop = forwardAmt ? `￥${formatMoneyAmount(forwardAmt)}` : "";
    const labelBottom = reverseAmt ? `￥${formatMoneyAmount(reverseAmt)}` : "";
    const edgeArrow = normalizeEdgeArrowValue(
      raw?.edgeArrow || raw?.arrow || (mode === "double" ? "both" : style.edgeArrow || "end"),
      mode === "double" ? "both" : "end"
    );
    return {
      ...collectGraphExtraFields(raw, EDGE_DISPLAY_RESERVED_FIELDS),
      id: String(raw?.id || `${source}=>${target}`),
      source,
      target,
      type: entry.isSelfLoop ? "self-loop" : "center-link",
      mode,
      label,
      labelTop,
      labelBottom,
      userCreated: !!raw?.userCreated,
      stroke: raw?.stroke || style.edgeColor || palette.edgeStroke,
      lineWidth: Number(raw?.lineWidth ?? raw?.line_width ?? style.edgeWidth) || 2,
      edgeDash: String(raw?.edgeDash || raw?.edge_dash || style.edgeDash || "solid"),
      edgeArrow,
      gap: 20,
      holePad: 2,
      outPad: 0.8,
      showArrow: edgeArrow !== "none",
      textColor: style.textColor || palette.edgeText,
      fontFamily: style.fontFamily || defaultFontFamily,
      fontSize: style.fontSize || 13,
      fontBold: coerceBool(style.fontBold, false),
      fontItalic: coerceBool(style.fontItalic, false),
      fontUnderline: coerceBool(style.fontUnderline, false),
      fontShadow: coerceBool(style.fontShadow, false),
      amount,
      count,
      in_amount: inAmt,
      out_amount: outAmt,
      forward_amount: forwardAmt,
      reverse_amount: reverseAmt,
      first_time: raw?.first_time || "",
      last_time: raw?.last_time || "",
      projection_edge: !!raw?.projection_edge,
    };
  }

  function buildGraphTxnNodeId(account, name, fallbackId) {
    const acct = String(account || "").trim();
    if (acct) return acct;
    const nodeName = String(name || "").trim();
    if (nodeName) return nodeName;
    return fallbackId || `txn-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  }

  function buildGraphTxnNodeDisplayModel(node, options = {}) {
    if (!node || typeof node !== "object") return null;
    const style = options?.style && typeof options.style === "object" ? options.style : {};
    const palette = resolveGraphThemePalette(options);
    const defaultFontFamily = String(options?.defaultFontFamily || "").trim() || "PingFang SC";
    const isSeed = !!options?.isSeed;
    const baseSize = style.nodeSize ?? 18;
    const seedSize = Math.max(baseSize + 4, 22);
    const r = Math.max(14, Math.min(30, Number(node?.r) || (isSeed ? seedSize : baseSize)));
    const iconSymbol = style.iconSymbol || "";
    const icon = iconSymbol ? "" : isSeed ? "★" : "";
    return {
      ...node,
      type: "hollow-entity",
      r,
      stroke: style.nodeColor || (isSeed ? "#1f6feb" : palette.nodeStroke),
      fill: style.nodeFill || palette.nodeFill,
      lineWidth: style.nodeWidth || (isSeed ? 3 : 2),
      textColor: style.textColor || palette.nodeText,
      subTextColor: style.textColor || palette.nodeSubText,
      icon,
      iconSymbol,
      iconSize: style.iconSize || 16,
      nodeShape: style.nodeShape || "circle",
      fontFamily: style.fontFamily || defaultFontFamily,
      fontSize: style.fontSize || 13,
      fontBold: coerceBool(style.fontBold, false),
      fontItalic: coerceBool(style.fontItalic, false),
      fontUnderline: coerceBool(style.fontUnderline, false),
      fontShadow: coerceBool(style.fontShadow, false),
      nodeShadow: coerceBool(style.nodeShadow, false),
    };
  }

  function buildGraphTxnEdgeDisplayModel(edge, options = {}) {
    if (!edge || typeof edge !== "object") return null;
    const style = options?.style && typeof options.style === "object" ? options.style : {};
    const palette = resolveGraphThemePalette(options);
    const defaultFontFamily = String(options?.defaultFontFamily || "").trim() || "PingFang SC";
    const isSelfLoop = edge.source === edge.target;
    return {
      ...edge,
      type: isSelfLoop ? "self-loop" : "center-link",
      mode: "single",
      edgeArrow: "end",
      showArrow: true,
      stroke: style.edgeColor || palette.edgeStroke,
      lineWidth: style.edgeWidth || 2,
      edgeDash: style.edgeDash || "solid",
      textColor: style.textColor || palette.edgeText,
      fontFamily: style.fontFamily || defaultFontFamily,
      fontSize: style.fontSize || 13,
      fontBold: coerceBool(style.fontBold, false),
      fontItalic: coerceBool(style.fontItalic, false),
      fontUnderline: coerceBool(style.fontUnderline, false),
      fontShadow: coerceBool(style.fontShadow, false),
    };
  }

  function formatGraphTxnEdgeLabel(amount, timeStr) {
    const amt = Number(amount) || 0;
    const amountText = amt ? `￥${formatMoneyAmount(Math.abs(amt))}` : "";
    const timeText = timeStr ? String(timeStr) : "";
    return `${amountText}${timeText ? ` · ${timeText}` : ""}`.trim();
  }

  function buildGraphTxnNodeModel(input = {}, options = {}) {
    const acct = String(input?.account || "").trim();
    const name = String(input?.name || "").trim();
    const nodeId = input?.id || buildGraphTxnNodeId(acct, name);
    const displayRaw = acct ? `${acct}` : "";
    const displayIds = normalizeDisplayIds(displayRaw);
    const base = {
      id: nodeId,
      title: name || acct || nodeId,
      name,
      displayId: displayRaw || acct || nodeId,
      displayIdRaw: displayRaw || acct || nodeId,
      displayIds,
      drillable: !!input?.drillable,
    };
    return buildGraphTxnNodeDisplayModel(base, { ...options, isSeed: false });
  }

  function buildGraphTxnEdgeModel(input = {}, options = {}) {
    const label = formatGraphTxnEdgeLabel(input?.amount, input?.time);
    return buildGraphTxnEdgeDisplayModel(
      {
        id: input?.id,
        source: input?.source,
        target: input?.target,
        amount: input?.amount,
        label,
        first_time: input?.time || "",
        last_time: input?.time || "",
      },
      options
    );
  }

  function applyGraphDisplayFilters(payload, options = {}) {
    let nodes = Array.isArray(payload?.nodes) ? payload.nodes : [];
    let edges = Array.isArray(payload?.edges) ? payload.edges : [];
    const applyUnknownName = String(options.focusKeyType || "").toLowerCase() === "account";
    if (applyUnknownName) nodes.forEach((node) => applyUnknownNameLabel(node, true));

    const { focusId, focusIds } = resolveGraphFocus(nodes, options);
    const graphMode = String(options.graphMode || "").toLowerCase();
    const isStatsBatchFocus = !!options.isStatsBatchFocus;

    if (graphMode === "relation") {
      const isStatsFocusRelation =
        !!options.isStatsSource &&
        !!options.focusOnly &&
        focusIds &&
        focusIds.size &&
        nodes.length &&
        !nodes.every((node) => focusIds.has(node.id));
      if (!isStatsBatchFocus && isStatsFocusRelation) {
        const used = new Set();
        edges = edges.filter((edge) => {
          const keep = focusIds.has(edge.source) || focusIds.has(edge.target);
          if (!keep) return false;
          used.add(edge.source);
          used.add(edge.target);
          return true;
        });
        nodes = used.size
          ? nodes.filter((node) => used.has(node.id) || focusIds.has(node.id))
          : nodes.filter((node) => focusIds.has(node.id));
      }
    } else if (graphMode === "net") {
      const allNodesFocused =
        focusIds &&
        focusIds.size &&
        nodes.length &&
        nodes.every((node) => focusIds.has(node.id));
      if (!isStatsBatchFocus && !allNodesFocused) {
        const kept = [];
        const used = new Set();
        edges.forEach((edge) => {
          let mainId = "";
          if (focusIds && focusIds.size) {
            const sourceIsFocus = focusIds.has(edge.source);
            const targetIsFocus = focusIds.has(edge.target);
            if (sourceIsFocus && targetIsFocus) return;
            if (!sourceIsFocus && !targetIsFocus) return;
            mainId = sourceIsFocus ? edge.source : edge.target;
          } else if (focusId) {
            if (edge.source !== focusId && edge.target !== focusId) return;
            mainId = focusId;
          }
          const { inToMain, outFromMain } = resolveNetMainAmounts(edge, mainId);
          const net = Number(inToMain) - Number(outFromMain);
          if (!net) return;
          const mainIsSource = mainId && edge.source === mainId;
          const mainIsTarget = mainId && edge.target === mainId;
          if (!mainIsSource && !mainIsTarget) return;
          edge.mode = "single";
          edge.edgeArrow = net > 0 ? (mainIsTarget ? "end" : "start") : mainIsSource ? "end" : "start";
          edge.showArrow = true;
          edge.label = `￥${formatMoneyAmount(Math.abs(net))}`;
          edge.labelTop = "";
          edge.labelBottom = "";
          kept.push(edge);
          used.add(edge.source);
          used.add(edge.target);
        });
        edges = kept;
        nodes = nodes.filter((node) =>
          used.has(node.id) || (focusIds.size ? focusIds.has(node.id) : node.id === focusId)
        );
      }
    }

    return { nodes, edges, focusId, focusIds: Array.from(focusIds || []) };
  }

  root.AnalytixGraphDataModel = Object.freeze({
    isUnknownAccountLabel,
    stripDisplayIdPrefix,
    normalizeDisplayIds,
    formatDisplayIdLabel,
    collectNodeDisplayIds,
    normalizeNodeDisplay,
    cloneGraphData,
    cloneFlowSummaryBoundary,
    cloneResultSnapshotRef,
    cloneGraphProjection,
    normalizeSnapshotGraphPayload,
    buildGraphSnapshotData,
    normalizeRawGraphPayload,
    resolveGraphThemePalette,
    buildGraphNodeDisplayModel,
    buildGraphEdgeDisplayModel,
    buildGraphTxnNodeId,
    buildGraphTxnNodeModel,
    buildGraphTxnEdgeModel,
    applyGraphDisplayFilters,
  });
})();
