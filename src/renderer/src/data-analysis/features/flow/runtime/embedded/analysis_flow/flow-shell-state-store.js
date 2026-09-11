(() => {
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__ || null;
  if (
    !piiProjection ||
    typeof piiProjection.projectField !== "function" ||
    typeof piiProjection.projectDetected !== "function"
  ) {
    throw new Error("ordinary PII projection missing for flow shell state");
  }

  function initialPresentationGeneration() {
    const words = new Uint32Array(2);
    if (window.crypto && typeof window.crypto.getRandomValues === "function") {
      window.crypto.getRandomValues(words);
      const value = (words[0] & 0x000fffff) * 0x100000000 + words[1];
      return Number.isSafeInteger(value) && value > 0 ? value : 1;
    }
    return Math.max(1, Date.now() * 1024);
  }

  let nextPresentationGeneration = initialPresentationGeneration();

  function allocatePresentationGeneration() {
    nextPresentationGeneration += 1;
    if (!Number.isSafeInteger(nextPresentationGeneration) || nextPresentationGeneration <= 0) {
      nextPresentationGeneration = initialPresentationGeneration();
    }
    return nextPresentationGeneration;
  }

  function text(value) {
    return String(value == null ? "" : value);
  }

  function escapeHtml(value) {
    return text(value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function createFlowShellStateStore({ state = null, deps = {} } = {}) {
    let presentationFingerprint = "";
    let presentationGeneration = 0;
    const nextGeneration =
      typeof deps.allocatePresentationGeneration === "function"
        ? deps.allocatePresentationGeneration
        : allocatePresentationGeneration;
    const canvasShellAdapter = deps.canvasShellAdapter || null;
    const getSelectionState = typeof deps.getSelectionState === "function" ? deps.getSelectionState : () => ({ nodes: [] });
    const buildShellViews = typeof deps.buildShellViews === "function" ? deps.buildShellViews : () => [];
    const buildShellStyleState =
      typeof deps.buildShellStyleState === "function" ? deps.buildShellStyleState : () => ({});
    const buildShellOverlayState =
      typeof deps.buildShellOverlayState === "function" ? deps.buildShellOverlayState : () => ({});
    const buildProjectionShellState =
      typeof deps.buildProjectionShellState === "function" ? deps.buildProjectionShellState : () => ({});
    const buildGraphStatsSummary =
      typeof deps.buildGraphStatsSummary === "function"
        ? deps.buildGraphStatsSummary
        : () => ({
            contract: "FlowGraphSummaryV1",
            status: "unknown",
            factAnswerAllowed: false,
            layoutLabel: "布局",
            nodes: null,
            edges: null,
            amount: null,
            boundaryCode: "host_verification_missing",
          });
    const layoutPresetLabel = typeof deps.layoutPresetLabel === "function" ? deps.layoutPresetLabel : (value) => text(value || "compact");
    const hasAnalysisAmountFilter =
      typeof deps.hasAnalysisAmountFilter === "function" ? deps.hasAnalysisAmountFilter : () => false;
    const clampNumber =
      typeof deps.clampNumber === "function"
        ? deps.clampNumber
        : (value, min, max) => Math.max(min, Math.min(max, Number(value) || min));

    function projectIdentityMeta(value) {
      const raw = text(value).trim();
      if (!raw || raw === "无证件号" || raw === "未登记户名") return piiProjection.projectDetected(raw);
      return piiProjection.projectField("id_no", raw);
    }

    function projectTreeSearch(value) {
      const raw = text(value).trim();
      const canonical = raw.normalize("NFKC").replace(/\p{Cf}/gu, "");
      const compact = canonical.replace(/[\s\-‐‑‒–—―_/\\.·•]+/gu, "");
      return /^\d{8,}$/.test(compact)
        ? piiProjection.projectField("account_no", raw)
        : piiProjection.projectDetected(raw);
    }

    function mixPresentationFingerprint(hash, value) {
      const input = `${text(value).length}:${text(value)}\u0000`;
      let next = hash >>> 0;
      for (let index = 0; index < input.length; index += 1) {
        next ^= input.charCodeAt(index);
        next = Math.imul(next, 16777619) >>> 0;
      }
      return next;
    }

    function currentPresentationFingerprint() {
      const groups = Array.isArray(state?.treeData) ? state.treeData : [];
      let first = mixPresentationFingerprint(2166136261, state?.caseId);
      let second = mixPresentationFingerprint(2246822507, state?.tab);
      let nodeCount = 0;
      groups.forEach((group, groupIndex) => {
        first = mixPresentationFingerprint(first, groupIndex);
        first = mixPresentationFingerprint(first, group?.id);
        second = mixPresentationFingerprint(second, group?.id);
        nodeCount += 1;
        const items = Array.isArray(group?.items) ? group.items : [];
        items.forEach((item, itemIndex) => {
          first = mixPresentationFingerprint(first, item?.id);
          second = mixPresentationFingerprint(second, `${groupIndex}:${itemIndex}:${text(item?.id)}`);
          nodeCount += 1;
        });
      });
      return `${first.toString(16)}:${second.toString(16)}:${groups.length}:${nodeCount}`;
    }

    function ensurePresentationGeneration() {
      const fingerprint = currentPresentationFingerprint();
      if (fingerprint !== presentationFingerprint) {
        presentationFingerprint = fingerprint;
        const allocated = Number(nextGeneration());
        if (!Number.isSafeInteger(allocated) || allocated <= 0) {
          throw new Error("ordinary presentation generation allocator returned an invalid value");
        }
        presentationGeneration = allocated;
      }
      return presentationGeneration;
    }

    function groupPresentationPosition(groupIndex) {
      const groups = Array.isArray(state?.treeData) ? state.treeData : [];
      let position = 1;
      for (let index = 0; index < groupIndex; index += 1) {
        position += 1 + (Array.isArray(groups[index]?.items) ? groups[index].items.length : 0);
      }
      return position;
    }

    function groupPresentationId(groupIndex) {
      return `flow-tree-group-${ensurePresentationGeneration()}-${groupPresentationPosition(groupIndex)}`;
    }

    function accountPresentationId(groupIndex, itemIndex) {
      return `flow-tree-account-${ensurePresentationGeneration()}-${groupPresentationPosition(groupIndex) + itemIndex + 1}`;
    }

    function lookupPresentationId(kind, rawValue) {
      const raw = text(rawValue);
      if (!raw) return "";
      const groups = Array.isArray(state?.treeData) ? state.treeData : [];
      if (kind === "group") {
        const groupIndex = groups.findIndex((group) => text(group?.id) === raw);
        return groupIndex >= 0 ? groupPresentationId(groupIndex) : "";
      }
      if (kind === "account") {
        for (let groupIndex = 0; groupIndex < groups.length; groupIndex += 1) {
          const items = Array.isArray(groups[groupIndex]?.items) ? groups[groupIndex].items : [];
          const itemIndex = items.findIndex((item) => text(item?.id) === raw);
          if (itemIndex >= 0) return accountPresentationId(groupIndex, itemIndex);
        }
      }
      return "";
    }

    function lookupRawPresentationId(token) {
      const value = text(token);
      const generation = ensurePresentationGeneration();
      const groupMatch = value.match(/^flow-tree-group-(\d+)-(\d+)$/);
      if (groupMatch && Number(groupMatch[1]) === generation) {
        const groups = Array.isArray(state?.treeData) ? state.treeData : [];
        const position = Number(groupMatch[2]);
        for (let groupIndex = 0; groupIndex < groups.length; groupIndex += 1) {
          if (groupPresentationPosition(groupIndex) === position) return text(groups[groupIndex]?.id);
        }
        return "";
      }
      const accountMatch = value.match(/^flow-tree-account-(\d+)-(\d+)$/);
      if (accountMatch && Number(accountMatch[1]) === generation) {
        const groups = Array.isArray(state?.treeData) ? state.treeData : [];
        const position = Number(accountMatch[2]);
        for (let groupIndex = 0; groupIndex < groups.length; groupIndex += 1) {
          const itemIndex = position - groupPresentationPosition(groupIndex) - 1;
          const items = Array.isArray(groups[groupIndex]?.items) ? groups[groupIndex].items : [];
          if (itemIndex >= 0 && itemIndex < items.length) return text(items[itemIndex]?.id);
        }
        return "";
      }
      return "";
    }

    function cloneShellTreeData() {
      ensurePresentationGeneration();
      return (state?.treeData || []).map((group, groupIndex) => ({
        id: groupPresentationId(groupIndex),
        title:
          state?.tab === "byName" && String(group?.meta || "") === "未登记户名"
            ? piiProjection.projectField("account_no", group?.title || "")
            : piiProjection.projectDetected(group?.title || ""),
        meta:
          state?.tab === "byName" && String(group?.meta || "") !== "未登记户名"
            ? projectIdentityMeta(group?.meta)
            : piiProjection.projectDetected(group?.meta || ""),
        extra: piiProjection.projectDetected(group?.extra || ""),
        items: Array.isArray(group?.items)
          ? group.items.map((item, itemIndex) => ({
              id: accountPresentationId(groupIndex, itemIndex),
              title: piiProjection.projectField("account_no", item?.title || item?.id || ""),
              sub: piiProjection.projectDetected(item?.sub || ""),
            }))
          : [],
      }));
    }

    function buildLegacyTreeOuterHtml({ search = "", expandedIds = [], selectedIds = [], icons = {} } = {}) {
      const treeData = cloneShellTreeData();
      if (!treeData.length) {
        return `<div class="treeEmpty">${state?.caseId ? "对象范围尚未取得宿主证据" : "未选择案件"}</div>`;
      }

      const query = projectTreeSearch(search).trim().toLowerCase();
      const expanded = new Set(
        (Array.isArray(expandedIds) ? expandedIds : []).map((id) => lookupPresentationId("group", id)).filter(Boolean)
      );
      const selected = new Set(
        (Array.isArray(selectedIds) ? selectedIds : []).map((id) => lookupPresentationId("account", id)).filter(Boolean)
      );
      const chevronIcon = text(icons.chevron || "");
      const checkIcon = text(icons.check || "");

      return treeData
        .filter((group) => {
          const groupText = `${group.title} ${group.meta} ${group.extra || ""}`.toLowerCase();
          return (
            !query ||
            groupText.includes(query) ||
            (group.items || []).some((item) => `${item.title} ${item.sub}`.toLowerCase().includes(query))
          );
        })
        .map((group) => {
          const isExpanded = expanded.has(group.id);
          const groupText = `${group.title} ${group.meta} ${group.extra || ""}`.toLowerCase();
          const visibleItems = (group.items || []).filter(
            (item) => !query || `${item.title} ${item.sub}`.toLowerCase().includes(query) || groupText.includes(query)
          );
          const selectedCount = (group.items || []).filter((item) => selected.has(item.id)).length;
          const checked = group.items.length > 0 && selectedCount === group.items.length;
          const indeterminate = selectedCount > 0 && selectedCount < group.items.length;
          const checkboxStyle = indeterminate
            ? ' style="background:rgba(245,158,11,.12);border-color:rgba(245,158,11,.40)"'
            : "";
          const itemHtml = visibleItems
            .map(
              (item) =>
                `<div class="item" data-iid="${escapeHtml(item.id)}">` +
                `<div class="cb${selected.has(item.id) ? " checked" : ""}">${checkIcon}</div>` +
                `<div class="iMain"><div class="iTitle">${escapeHtml(item.title)}</div>` +
                `<div class="iSub">${escapeHtml(item.sub)}</div></div></div>`
            )
            .join("");
          return (
            `<div class="group${isExpanded ? " open" : ""}">` +
            `<div class="gHead" data-gid="${escapeHtml(group.id)}">` +
            `<div class="gArrow${isExpanded ? " open" : ""}">${chevronIcon}</div>` +
            `<div class="cb${checked ? " checked" : ""}"${checkboxStyle}>${checkIcon}</div>` +
            `<div class="gMain"><div class="gTitle">${escapeHtml(group.title)}</div>` +
            `<div class="gMeta">${escapeHtml(group.meta + (group.extra ? ` · ${group.extra}` : ""))}</div></div>` +
            `<div class="gCount">${group.items.length}</div></div>` +
            `<div class="gBody${isExpanded ? " open" : ""}">${itemHtml}</div></div>`
          );
        })
        .join("");
    }

    function build() {
      const viewItems = buildShellViews();
      const selectedGraphNodeCount = getSelectionState(state?.graph?.instance).nodes.length;
      const canvasChromeState =
        canvasShellAdapter && typeof canvasShellAdapter.buildCanvasChromeState === "function"
          ? canvasShellAdapter.buildCanvasChromeState({
              graphData: state?.graph?.data || null,
              layoutPreset: state?.layoutPreset || "compact",
              graphSearchQuery: state?.graphSearchQuery || "",
              graphSearchFocusToken: state?.graphSearchFocusToken || 0,
              selectedGraphNodeCount,
              views: viewItems,
              activeViewId: state?.activeViewId || "",
            })
          : null;
      const graphStats =
        canvasChromeState && canvasChromeState.graphStats
          ? canvasChromeState.graphStats
          : buildGraphStatsSummary(state?.graph?.data || null, state?.layoutPreset || "compact");
      const shellState = {
        caseId: piiProjection.projectDetected(state?.caseId || ""),
        caseName: piiProjection.projectDetected(state?.caseName || ""),
        leftCollapsed: !!state?.leftCollapsed,
        tab: state?.tab === "byCard" ? "byCard" : "byName",
        search: projectTreeSearch(state?.search || ""),
        treeSemanticStatus:
          state?.treeSemanticStatus === "source_unavailable"
            ? "source_unavailable"
            : state?.treeSemanticStatus === "blocked"
              ? "blocked"
              : "uninitialized",
        graphSearch:
          canvasChromeState && canvasChromeState.graphSearch
            ? canvasChromeState.graphSearch
            : {
                query: projectTreeSearch(state?.graphSearchQuery || ""),
                focusToken: Math.max(0, Number(state?.graphSearchFocusToken) || 0),
              },
        treeData: cloneShellTreeData(),
        expandedIds: Array.from(state?.expanded || []).map((id) => lookupPresentationId("group", id)).filter(Boolean),
        selectedIds: Array.from(state?.selected || []).map((id) => lookupPresentationId("account", id)).filter(Boolean),
        selectedCount: state?.selected?.size || 0,
        filters: {
          dir: state?.dir === "in" || state?.dir === "out" ? state.dir : "all",
          hop: clampNumber(Number(state?.hop) || 1, 1, 3),
          minAmount: Math.max(0, Number(state?.minAmount) || 0),
          maxEdges: Math.max(50, Number(state?.maxEdges) || 800),
        },
        layout: {
          preset: String(state?.layoutPreset || "compact"),
          selected: !!state?.layoutSelected,
          direction: { ...(state?.layoutDirection || {}) },
          edgeRouting: { ...(state?.layoutEdgeRouting || {}) },
        },
        analysis: {
          encodeWidth: !!state?.analysis?.encodeWidth,
          encodeColor: !!state?.analysis?.encodeColor,
          nodeScale: !!state?.analysis?.nodeScale,
          filterActive: hasAnalysisAmountFilter(),
          filterMin: Number.isFinite(state?.analysis?.filterMin) ? Number(state.analysis.filterMin) : null,
          filterMax: Number.isFinite(state?.analysis?.filterMax) ? Number(state.analysis.filterMax) : null,
          collapseChildren: !!state?.analysis?.collapseChildren,
        },
        graphStats:
          graphStats && typeof graphStats === "object" && !Array.isArray(graphStats)
            ? { ...graphStats }
            : {
                contract: "FlowGraphSummaryV1",
                status: "unknown",
                factAnswerAllowed: false,
                layoutLabel: String(layoutPresetLabel(state?.layoutPreset || "compact")),
                nodes: null,
                edges: null,
                amount: null,
                boundaryCode: "host_verification_missing",
              },
        ops: {
          graphLoading: !!state?.graphLoading,
          edgeDetail: !!state?.edgeDetail,
          canUndo: (state?.undoStack || []).length > 0,
          canRedo: !!state?.initialStyleSnapshot,
        },
        style: buildShellStyleState(),
        projection:
          canvasChromeState && canvasChromeState.projection ? canvasChromeState.projection : buildProjectionShellState(),
        views: Array.isArray(canvasChromeState?.views) ? canvasChromeState.views : viewItems,
        activeViewId: String(state?.activeViewId || ""),
        overlay: buildShellOverlayState(),
      };
      if (canvasShellAdapter && typeof canvasShellAdapter.buildCanvasShellState === "function") {
        return canvasShellAdapter.buildCanvasShellState(shellState);
      }
      return shellState;
    }

    return {
      build,
      buildLegacyTreeOuterHtml,
      cloneShellTreeData,
      presentationIdFor(kind, value) {
        return lookupPresentationId(kind, value);
      },
      projectTreeSearch,
      resolvePresentationId(value) {
        return lookupRawPresentationId(value);
      },
    };
  }

  window.__ANALYTIX_FLOW_SHELL_STATE_STORE__ = {
    createFlowShellStateStore,
  };
})();
