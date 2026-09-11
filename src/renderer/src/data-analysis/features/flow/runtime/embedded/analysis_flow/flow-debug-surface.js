(() => {
  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function asNumber(value, fallback = 0) {
    const next = Number(value);
    return Number.isFinite(next) ? next : fallback;
  }

  function clonePlainObject(value, fallback = null) {
    if (!value || typeof value !== "object") return fallback;
    try {
      return JSON.parse(JSON.stringify(value));
    } catch (_error) {
      return fallback;
    }
  }

  function createFlowDebugSurface({
    state = null,
    ensureGraph = null,
    getGraphEngine = null,
    ensureLayoutSemanticState = null,
    getLayoutEngine = null,
    recordTextAtlasMetrics = null,
    flowObservabilityAdapter = null,
    getNetworkSectorPlacementCacheSnapshot = null,
  } = {}) {
    function getLayoutSnapshot() {
      const nodes = Array.isArray(state?.graph?.data?.nodes) ? state.graph.data.nodes : [];
      const semanticState =
        typeof ensureLayoutSemanticState === "function" ? ensureLayoutSemanticState() : { layoutPlanCache: {}, clusterSlotBySignature: {} };
      const engine = typeof getLayoutEngine === "function" ? getLayoutEngine() : null;
      const busy =
        !!state?.layoutWorkerPending ||
        !!state?.graphLoading ||
        !!state?.edgeBatching ||
        !!state?.introAnimating ||
        !!state?.pendingFit ||
        Date.now() < Number(state?.introSuppressUntil || 0);
      return {
        preset: String(state?.layoutPreset || ""),
        selected: !!state?.layoutSelected,
        busy,
        direction: {
          hierarchy: String(state?.layoutDirection?.hierarchy || ""),
          flow: String(state?.layoutDirection?.flow || ""),
        },
        counts: {
          nodes: nodes.length,
          edges: Array.isArray(state?.graph?.data?.edges) ? state.graph.data.edges.length : 0,
        },
        mutation: {
          graphSeq: asNumber(state?.graphMutationSeq, 0),
          directRenderSeq: asNumber(state?.directRenderSeq, 0),
          lastReason: text(state?.lastMutationReason),
          lastAt: asNumber(state?.lastMutationAt, 0),
        },
        worker: {
          active: !!state?.layoutWorker,
          pending: !!state?.layoutWorkerPending,
          stage: String(state?.layoutWorkerStage || ""),
          seq: Number(state?.layoutWorkerPending?.seq || 0),
          disabledReason: String(state?.layoutWorkerDisabledReason || ""),
          traceId: text(state?.layoutWorkerPending?.traceId || state?.perfMetrics?.layoutWorker?.lastTraceId),
        },
        contract: {
          version: String(engine?.LAYOUT_CONTRACT_VERSION || ""),
          algoVersion: String(engine?.LAYOUT_ALGO_VERSION || ""),
          planCacheSize: Object.keys(semanticState.layoutPlanCache || {}).length,
          planCacheStats: { ...(semanticState.layoutPlanCacheStats || {}) },
          slotCacheSize: Object.keys(semanticState.clusterSlotBySignature || {}).length,
          lastAppliedLayoutCacheKey: String(state?.lastAppliedLayoutCacheKey || ""),
        },
        buildGraphPerf: clonePlainObject(state?.lastBuildGraphPerf, null),
        network: {
          planMode: text(state?.networkLayout?.lastPlanMode),
          planQuality:
            state?.networkLayout?.lastPlanQuality && typeof state.networkLayout.lastPlanQuality === "object"
              ? { ...state.networkLayout.lastPlanQuality }
              : null,
          planSampleQuality:
            state?.networkLayout?.lastPlanSampleQuality && typeof state.networkLayout.lastPlanSampleQuality === "object"
              ? { ...state.networkLayout.lastPlanSampleQuality }
              : null,
          planCommunityQuality: Array.isArray(state?.networkLayout?.lastPlanCommunityQuality)
            ? state.networkLayout.lastPlanCommunityQuality.map((row) => ({ ...(row || {}) }))
            : [],
          planSupergraph:
            state?.networkLayout?.lastPlanSupergraph && typeof state.networkLayout.lastPlanSupergraph === "object"
              ? clonePlainObject(state.networkLayout.lastPlanSupergraph, null)
              : null,
          planGraphSignature: text(state?.networkLayout?.lastPlanGraphSignature),
          planAttempt:
            state?.networkLayout?.lastPlanAttempt && typeof state.networkLayout.lastPlanAttempt === "object"
              ? { ...state.networkLayout.lastPlanAttempt }
              : null,
          planDrop:
            state?.networkLayout?.lastPlanDrop && typeof state.networkLayout.lastPlanDrop === "object"
              ? { ...state.networkLayout.lastPlanDrop }
              : null,
          communityQualityAttempt:
            state?.networkLayout?.lastCommunityQualityAttempt &&
            typeof state.networkLayout.lastCommunityQualityAttempt === "object"
              ? { ...state.networkLayout.lastCommunityQualityAttempt }
              : null,
          communityQualityDrop:
            state?.networkLayout?.lastCommunityQualityDrop &&
            typeof state.networkLayout.lastCommunityQualityDrop === "object"
              ? { ...state.networkLayout.lastCommunityQualityDrop }
              : null,
          communityQualityStaleDrop:
            state?.networkLayout?.lastCommunityQualityStaleDrop &&
            typeof state.networkLayout.lastCommunityQualityStaleDrop === "object"
              ? { ...state.networkLayout.lastCommunityQualityStaleDrop }
              : null,
          hiddenNodeCount: asNumber(state?.networkLayout?.hiddenNodeCount, 0),
          hiddenEdgeCount: asNumber(state?.networkLayout?.hiddenEdgeCount, 0),
          viewportQuality:
            state?.networkLayout?.lastViewportQuality && typeof state.networkLayout.lastViewportQuality === "object"
              ? { ...state.networkLayout.lastViewportQuality }
              : null,
          viewportRefine: state?.networkLayout?.lastViewportRefine
            ? { ...state.networkLayout.lastViewportRefine }
            : null,
          perf: clonePlainObject(state?.networkLayout?.perf, null),
        },
        nodes: nodes
          .map((node) => ({
            id: String(node?.id || ""),
            x: Number(Number(node?.x || 0).toFixed(3)),
            y: Number(Number(node?.y || 0).toFixed(3)),
            layoutBand: String(node?.layoutBand || ""),
            layoutLevel: Number.isFinite(Number(node?.layoutLevel)) ? Number(node.layoutLevel) : null,
          }))
          .sort((left, right) => left.id.localeCompare(right.id)),
      };
    }

    function getRendererSnapshot() {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      const graphEngine = typeof getGraphEngine === "function" ? getGraphEngine() : null;
      const info = graph && typeof graph.getDebugInfo === "function" ? graph.getDebugInfo() : {};
      const textInfo = info && typeof info.text === "object" && info.text ? info.text : {};
      return {
        renderer: String(info?.renderer || ""),
        engineNamespace: window.AnalytixGraphEngine && graphEngine === window.AnalytixGraphEngine ? "AnalytixGraphEngine" : "",
        usingAnalytixWebgl: String(getGraphEngine?.()?.version || "").startsWith("AnalytixWebGL"),
        text: {
          gpu: !!textInfo.gpu,
          active: !!textInfo.active,
          mode: String(textInfo.mode || ""),
          quality: String(textInfo.quality || ""),
          backend: String(textInfo.backend || ""),
          policyLocked: !!textInfo.policyLocked,
          textFallbackAllowed: !!textInfo.textFallbackAllowed,
          labels: Number(textInfo.labels || 0),
          sets: Number(textInfo.sets || 0),
          atlases: Number(textInfo.atlases || 0),
          glyphs: Number(textInfo.glyphs || 0),
          textureBytes: Number(textInfo.textureBytes || 0),
        },
      };
    }

    function cloneSelectedGraphStyle(item, type = "") {
      const model = item?.getModel ? item.getModel() || {} : item || {};
      return {
        type: text(type || item?.getType?.()),
        id: text(model.id || model.edge_id || item?.getID?.()),
        title: text(model.title),
        displayId: text(model.displayId || model.display_id),
        displayIdRaw: text(model.displayIdRaw || model.display_id_raw),
        displayIds: Array.isArray(model.displayIds)
          ? model.displayIds.map(text).filter(Boolean)
          : Array.isArray(model.display_ids)
            ? model.display_ids.map(text).filter(Boolean)
            : [],
        source: text(model.source),
        target: text(model.target),
        x: asNumber(model.x, 0),
        y: asNumber(model.y, 0),
        label: text(model.label),
        labelTop: text(model.labelTop),
        labelBottom: text(model.labelBottom),
        detailLabel: !!model.detailLabel,
        stroke: text(model.stroke),
        fill: text(model.fill),
        textColor: text(model.textColor),
        fontFamily: text(model.fontFamily),
        fontSize: asNumber(model.fontSize, 0),
        fontBold: !!model.fontBold,
        fontItalic: !!model.fontItalic,
        fontUnderline: !!model.fontUnderline,
        fontShadow: !!model.fontShadow,
        lineWidth: asNumber(model.lineWidth, 0),
        edgeDash: Array.isArray(model.edgeDash) ? model.edgeDash.slice() : text(model.edgeDash),
        edgeArrow: text(model.edgeArrow),
        mode: text(model.mode),
        r: asNumber(model.r, 0),
        nodeShape: text(model.nodeShape),
        iconSymbol: text(model.iconSymbol),
        groupTags: Array.isArray(model.groupTags) ? model.groupTags.slice() : [],
        userCreated: !!model.userCreated,
      };
    }

    function getGraphSelectionSnapshot() {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      const allNodes = graph?.getNodes?.() || [];
      const allEdges = graph?.getEdges?.() || [];
      const dataNodes = Array.isArray(state?.graph?.data?.nodes) ? state.graph.data.nodes : [];
      const dataEdges = Array.isArray(state?.graph?.data?.edges) ? state.graph.data.edges : [];
      const selectedNodes = graph?.findAllByState?.("node", "selected") || [];
      const selectedEdges = graph?.findAllByState?.("edge", "selected") || [];
      return {
        graphCounts: {
          nodes: Math.max(allNodes.length, dataNodes.length),
          edges: Math.max(allEdges.length, dataEdges.length),
        },
        runtimeCounts: {
          nodes: allNodes.length,
          edges: allEdges.length,
        },
        counts: {
          nodes: selectedNodes.length,
          edges: selectedEdges.length,
          total: selectedNodes.length + selectedEdges.length,
        },
        availableNodes: (allNodes.length ? allNodes : dataNodes).slice(0, 12).map((item) => cloneSelectedGraphStyle(item, "node")),
        availableEdges: (allEdges.length ? allEdges : dataEdges).slice(0, 12).map((item) => cloneSelectedGraphStyle(item, "edge")),
        edgePairs: (allEdges.length ? allEdges : dataEdges)
          .map((item) => {
            const model = item?.getModel ? item.getModel() || {} : item || {};
            const sourceModel = item?.getSource?.()?.getModel?.() || {};
            const targetModel = item?.getTarget?.()?.getModel?.() || {};
            return {
              source: text(model.source || sourceModel.id),
              target: text(model.target || targetModel.id),
            };
          })
          .filter((pair) => pair.source && pair.target),
        nodes: selectedNodes.slice(0, 32).map((item) => cloneSelectedGraphStyle(item, "node")),
        edges: selectedEdges.slice(0, 32).map((item) => cloneSelectedGraphStyle(item, "edge")),
      };
    }

    function getGraphItemStyleSnapshot(ids = []) {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      const wanted = new Set(
        (Array.isArray(ids) ? ids : [])
          .map((id) => text(id))
          .filter(Boolean)
      );
      if (!wanted.size) {
        return { nodes: [], edges: [] };
      }
      const dataNodes = Array.isArray(state?.graph?.data?.nodes) ? state.graph.data.nodes : [];
      const dataEdges = Array.isArray(state?.graph?.data?.edges) ? state.graph.data.edges : [];
      const nodes = [];
      const edges = [];
      wanted.forEach((id) => {
        const runtimeItem = graph?.findById?.(id);
        if (runtimeItem) {
          const type = text(runtimeItem.getType?.());
          if (type === "edge") {
            edges.push(cloneSelectedGraphStyle(runtimeItem, "edge"));
          } else {
            nodes.push(cloneSelectedGraphStyle(runtimeItem, "node"));
          }
          return;
        }
        const dataNode = dataNodes.find((node) => text(node?.id) === id);
        if (dataNode) {
          nodes.push(cloneSelectedGraphStyle(dataNode, "node"));
          return;
        }
        const dataEdge = dataEdges.find((edge) => text(edge?.id) === id || text(edge?.edge_id) === id);
        if (dataEdge) {
          edges.push(cloneSelectedGraphStyle(dataEdge, "edge"));
        }
      });
      return { nodes, edges };
    }

    function summarizeEdgePresentation(rows = []) {
      const items = (Array.isArray(rows) ? rows : [])
        .map((item) => (item?.getModel ? item.getModel() || {} : item || {}))
        .filter((item) => item && typeof item === "object");
      const hasLabel = (model) => text(model.label || model.labelTop || model.labelBottom).length > 0;
      const isWeakStyled = (model) => {
        const stroke = text(model.stroke);
        const lineWidth = asNumber(model.lineWidth, 0);
        return stroke.includes("148,163,184") || (lineWidth > 0 && lineWidth <= 0.25 && !hasLabel(model));
      };
      return {
        total: items.length,
        denseStyled: items.filter((model) => text(model.__networkDenseStyled)).length,
        weakStyled: items.filter(isWeakStyled).length,
        hiddenLabels: items.filter(
          (model) => model.edgeLabelHidden === true || text(model.layoutEdgeLabelTier) === "hidden"
        ).length,
        labeled: items.filter(hasLabel).length,
        arrowed: items.filter((model) => !!model.showArrow).length,
      };
    }

    function getGraphEdgePresentationSnapshot() {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      const runtimeEdges = graph?.getEdges?.() || [];
      const dataEdges = Array.isArray(state?.graph?.data?.edges) ? state.graph.data.edges : [];
      return {
        layoutPreset: text(state?.layoutPreset),
        runtime: summarizeEdgePresentation(runtimeEdges),
        data: summarizeEdgePresentation(dataEdges),
      };
    }

    function edgeEndpoint(value) {
      if (value && typeof value === "object") return text(value.id || value.key);
      return text(value);
    }

    function edgeWeight(model = {}) {
      return Math.max(
        0,
        asNumber(model.weight, 0) ||
          asNumber(model.amount, 0) ||
          asNumber(model.total_amount, 0) ||
          asNumber(model.totalAmount, 0) ||
          asNumber(model.count, 0)
      );
    }

    function countHoverStyledEdges(edgeItems = []) {
      return (Array.isArray(edgeItems) ? edgeItems : []).filter((item) => {
        const model = item?.getModel?.() || {};
        return model.__networkDenseHoverReveal === true || text(model.__networkDenseStyled) === "hover";
      }).length;
    }

    function triggerNetworkDenseHoverRevealForDebug() {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      if (!graph) {
        return {
          ok: false,
          reason: "missing-graph",
          mode: "",
          nodeId: "",
          runtimeEdges: 0,
          weakCandidateEdges: 0,
          incidentWeakCandidateEdges: 0,
          revealEdges: 0,
          hoverStyledEdges: 0,
          beforeHoverStyledEdges: 0,
          maxHoverLineWidth: 0,
        };
      }
      const edgeItems = graph.getEdges?.() || [];
      const nodeItems = graph.getNodes?.() || [];
      const nodeIds = new Set(nodeItems.map((item) => edgeEndpoint(item?.getModel?.()?.id)).filter(Boolean));
      const weakRows = edgeItems
        .map((item, index) => {
          const model = item?.getModel?.() || {};
          const source = edgeEndpoint(model.source);
          const target = edgeEndpoint(model.target);
          const denseStyled = text(model.__networkDenseStyled);
          const weakLine = asNumber(model.lineWidth, 0) > 0 && asNumber(model.lineWidth, 0) <= 0.45;
          const hiddenBySkeleton = !!model.layoutHiddenBySkeleton || text(model.layoutEdgeTier) === "hidden";
          const weak = denseStyled === "weak" || weakLine || hiddenBySkeleton;
          return { index, source, target, weak, weight: edgeWeight(model) };
        })
        .filter((row) => row.weak && row.source && row.target && row.source !== row.target);
      const endpoints = new Map();
      weakRows.forEach((row) => {
        [row.source, row.target].forEach((id) => {
          if (!nodeIds.has(id)) return;
          const current = endpoints.get(id) || { id, count: 0, weight: 0, firstIndex: row.index };
          current.count += 1;
          current.weight += row.weight;
          current.firstIndex = Math.min(current.firstIndex, row.index);
          endpoints.set(id, current);
        });
      });
      const candidate = Array.from(endpoints.values()).sort(
        (left, right) => right.count - left.count || right.weight - left.weight || left.firstIndex - right.firstIndex
      )[0];
      if (!candidate) {
        return {
          ok: false,
          reason: "missing-weak-incident-node",
          mode: text(state?.networkLayout?.lastPlanMode),
          nodeId: "",
          runtimeEdges: edgeItems.length,
          weakCandidateEdges: weakRows.length,
          incidentWeakCandidateEdges: 0,
          revealEdges: 0,
          hoverStyledEdges: countHoverStyledEdges(edgeItems),
          beforeHoverStyledEdges: countHoverStyledEdges(edgeItems),
          maxHoverLineWidth: 0,
        };
      }
      const nodeItem =
        graph.findById?.(candidate.id) ||
        nodeItems.find((item) => edgeEndpoint(item?.getModel?.()?.id) === candidate.id) ||
        null;
      const beforeHoverStyledEdges = countHoverStyledEdges(edgeItems);
      graph.emit?.("node:mouseenter", { item: nodeItem, target: nodeItem, synthetic: true });
      const reveal =
        state?.networkLayout?.lastDenseHoverReveal && typeof state.networkLayout.lastDenseHoverReveal === "object"
          ? state.networkLayout.lastDenseHoverReveal
          : {};
      const hoverStyledEdges = countHoverStyledEdges(edgeItems);
      const hoverLineWidths = edgeItems
        .map((item) => item?.getModel?.() || {})
        .filter((model) => model.__networkDenseHoverReveal === true || text(model.__networkDenseStyled) === "hover")
        .map((model) => asNumber(model.lineWidth, 0));
      return {
        ok: hoverStyledEdges > beforeHoverStyledEdges && asNumber(reveal.edges, 0) > 0,
        reason: "",
        mode: text(state?.networkLayout?.lastPlanMode),
        nodeId: candidate.id,
        runtimeEdges: edgeItems.length,
        weakCandidateEdges: weakRows.length,
        incidentWeakCandidateEdges: candidate.count,
        revealEdges: asNumber(reveal.edges, 0),
        hoverStyledEdges,
        beforeHoverStyledEdges,
        maxHoverLineWidth: Math.max(0, ...hoverLineWidths),
      };
    }

    function restoreNetworkDenseHoverRevealForDebug(nodeId = "") {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      const edgeItems = graph?.getEdges?.() || [];
      const nodeItem = text(nodeId) ? graph?.findById?.(text(nodeId)) || null : null;
      if (nodeItem) {
        graph?.emit?.("node:mouseleave", { item: nodeItem, target: nodeItem, synthetic: true });
      } else {
        graph?.emit?.("canvas:mouseleave", { synthetic: true });
      }
      const restore =
        state?.networkLayout?.lastDenseHoverRevealRestore &&
        typeof state.networkLayout.lastDenseHoverRevealRestore === "object"
          ? state.networkLayout.lastDenseHoverRevealRestore
          : {};
      return {
        restoreEdges: asNumber(restore.edges, 0),
        hoverStyledEdges: countHoverStyledEdges(edgeItems),
      };
    }

    function summarizeScreenPointDistribution(points = [], width = 0, height = 0) {
      const canvasWidth = Math.max(1, asNumber(width, 0));
      const canvasHeight = Math.max(1, asNumber(height, 0));
      const visible = (Array.isArray(points) ? points : []).filter(
        (point) => point.x >= -80 && point.y >= -80 && point.x <= canvasWidth + 80 && point.y <= canvasHeight + 80
      );
      const columns = 12;
      const rows = 8;
      const cells = Array.from({ length: columns * rows }, () => 0);
      const quadrants = [0, 0, 0, 0];
      let centerCount = 0;
      let edgeCount = 0;
      let minX = Infinity;
      let maxX = -Infinity;
      let minY = Infinity;
      let maxY = -Infinity;
      visible.forEach((point) => {
        const unitX = Math.max(0, Math.min(1, (point.x + 0.5) / canvasWidth));
        const unitY = Math.max(0, Math.min(1, (point.y + 0.5) / canvasHeight));
        const column = Math.min(columns - 1, Math.max(0, Math.floor(unitX * columns)));
        const row = Math.min(rows - 1, Math.max(0, Math.floor(unitY * rows)));
        cells[row * columns + column] += 1;
        quadrants[(unitY >= 0.5 ? 2 : 0) + (unitX >= 0.5 ? 1 : 0)] += 1;
        if (unitX >= 0.25 && unitX <= 0.75 && unitY >= 0.22 && unitY <= 0.78) centerCount += 1;
        if (unitX <= 0.12 || unitX >= 0.88 || unitY <= 0.12 || unitY >= 0.88) edgeCount += 1;
        minX = Math.min(minX, point.x);
        maxX = Math.max(maxX, point.x);
        minY = Math.min(minY, point.y);
        maxY = Math.max(maxY, point.y);
      });
      const visibleCount = visible.length;
      const occupiedCellCount = cells.filter(Boolean).length;
      const maxCellPointCount = Math.max(0, ...cells);
      const bboxWidth = Number.isFinite(minX) ? Math.max(0, Math.min(canvasWidth, maxX) - Math.max(0, minX)) : 0;
      const bboxHeight = Number.isFinite(minY) ? Math.max(0, Math.min(canvasHeight, maxY) - Math.max(0, minY)) : 0;
      const quadrantRatios = quadrants.map((count) => count / Math.max(1, visibleCount));
      return {
        totalPoints: (Array.isArray(points) ? points : []).length,
        visiblePoints: visibleCount,
        gridColumns: columns,
        gridRows: rows,
        occupiedCellCount,
        occupiedCellRatio: occupiedCellCount / cells.length,
        maxCellPointCount,
        maxCellPointRatio: maxCellPointCount / Math.max(1, visibleCount),
        centerPointRatio: centerCount / Math.max(1, visibleCount),
        edgePointRatio: edgeCount / Math.max(1, visibleCount),
        bboxAreaRatio: (bboxWidth * bboxHeight) / Math.max(1, canvasWidth * canvasHeight),
        horizontalSpreadRatio: bboxWidth / canvasWidth,
        verticalSpreadRatio: bboxHeight / canvasHeight,
        quadrantMinRatio: Math.min(...quadrantRatios),
        quadrantMaxRatio: Math.max(...quadrantRatios),
      };
    }

    function summarizeScreenEdgeDistribution(edgeItems = [], nodePointById = new Map(), width = 0, height = 0) {
      const canvasWidth = Math.max(1, asNumber(width, 0));
      const canvasHeight = Math.max(1, asNumber(height, 0));
      const columns = 12;
      const rows = 8;
      const cells = Array.from({ length: columns * rows }, () => 0);
      const inViewport = (point, margin = 80) =>
        point &&
        point.x >= -margin &&
        point.y >= -margin &&
        point.x <= canvasWidth + margin &&
        point.y <= canvasHeight + margin;
      let drawableEdges = 0;
      let visibleEdges = 0;
      let strongVisibleEdges = 0;
      let weakVisibleEdges = 0;
      let hiddenBySkeletonEdges = 0;
      let lineWidthSum = 0;
      (Array.isArray(edgeItems) ? edgeItems : []).forEach((item) => {
        const model = item?.getModel?.() || item || {};
        const source = edgeEndpoint(model.source);
        const target = edgeEndpoint(model.target);
        const sourcePoint = nodePointById.get(source);
        const targetPoint = nodePointById.get(target);
        if (!sourcePoint || !targetPoint) return;
        drawableEdges += 1;
        const midPoint = {
          x: (sourcePoint.x + targetPoint.x) / 2,
          y: (sourcePoint.y + targetPoint.y) / 2,
        };
        const visible = inViewport(sourcePoint) || inViewport(targetPoint) || inViewport(midPoint, 20);
        const lineWidth = asNumber(model.lineWidth, 0);
        const hiddenBySkeleton =
          !!model.layoutHiddenBySkeleton || text(model.layoutEdgeTier) === "hidden" || lineWidth <= 0.05;
        const weak =
          hiddenBySkeleton ||
          text(model.__networkDenseStyled) === "weak" ||
          (lineWidth > 0 && lineWidth <= 0.3);
        if (hiddenBySkeleton) hiddenBySkeletonEdges += 1;
        if (!visible || lineWidth <= 0.05) return;
        visibleEdges += 1;
        lineWidthSum += lineWidth;
        if (weak) weakVisibleEdges += 1;
        else strongVisibleEdges += 1;
        const steps = 5;
        for (let step = 0; step <= steps; step += 1) {
          const t = step / steps;
          const x = sourcePoint.x + (targetPoint.x - sourcePoint.x) * t;
          const y = sourcePoint.y + (targetPoint.y - sourcePoint.y) * t;
          if (!inViewport({ x, y }, 0)) continue;
          const unitX = Math.max(0, Math.min(1, (x + 0.5) / canvasWidth));
          const unitY = Math.max(0, Math.min(1, (y + 0.5) / canvasHeight));
          const column = Math.min(columns - 1, Math.max(0, Math.floor(unitX * columns)));
          const row = Math.min(rows - 1, Math.max(0, Math.floor(unitY * rows)));
          cells[row * columns + column] += 1;
        }
      });
      const occupiedCellCount = cells.filter(Boolean).length;
      const maxCellEdgeSamples = Math.max(0, ...cells);
      return {
        totalEdges: (Array.isArray(edgeItems) ? edgeItems : []).length,
        drawableEdges,
        visibleEdges,
        strongVisibleEdges,
        weakVisibleEdges,
        hiddenBySkeletonEdges,
        visibleEdgeRatio: visibleEdges / Math.max(1, drawableEdges),
        strongVisibleEdgeRatio: strongVisibleEdges / Math.max(1, visibleEdges),
        occupiedCellCount,
        occupiedCellRatio: occupiedCellCount / cells.length,
        maxCellEdgeSampleRatio: maxCellEdgeSamples / Math.max(1, visibleEdges * 6),
        averageVisibleLineWidth: lineWidthSum / Math.max(1, visibleEdges),
      };
    }

    function getGraphViewportSnapshot() {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      if (!graph) {
        return {
          zoom: 0,
          width: 0,
          height: 0,
          runtimeCounts: { nodes: 0, edges: 0 },
          visibleNodes: 0,
          screenBounds: null,
          screenDistribution: null,
          edgeScreenDistribution: null,
        };
      }
      const width = asNumber(graph.get?.("width"), 0);
      const height = asNumber(graph.get?.("height"), 0);
      const nodes = Array.isArray(graph.getNodes?.()) ? graph.getNodes() : [];
      const nodePointById = new Map();
      const runtimeNodeIds = nodes
        .map((item) => text(item?.getModel?.()?.id))
        .filter(Boolean);
      const screenPoints =
        typeof graph.getCanvasByPoint === "function"
          ? nodes
              .map((item) => {
                const model = item?.getModel?.() || {};
                const id = text(model.id);
                const x = Number(model.x);
                const y = Number(model.y);
                if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
                try {
                  const point = graph.getCanvasByPoint(x, y);
                  if (point && Number.isFinite(point.x) && Number.isFinite(point.y)) {
                    if (id) nodePointById.set(id, point);
                    return point;
                  }
                  return null;
                } catch (e) {
                  return null;
                }
              })
              .filter(Boolean)
          : [];
      const visibleNodes = screenPoints.filter(
        (point) => point.x >= -80 && point.y >= -80 && point.x <= width + 80 && point.y <= height + 80
      ).length;
      const screenBounds = screenPoints.length
        ? {
            minX: Math.round(Math.min(...screenPoints.map((point) => point.x))),
            maxX: Math.round(Math.max(...screenPoints.map((point) => point.x))),
            minY: Math.round(Math.min(...screenPoints.map((point) => point.y))),
            maxY: Math.round(Math.max(...screenPoints.map((point) => point.y))),
          }
        : null;
      return {
        zoom: asNumber(graph.getZoom?.(), 0),
        width,
        height,
        runtimeCounts: {
          nodes: nodes.length,
          edges: Array.isArray(graph.getEdges?.()) ? graph.getEdges().length : 0,
        },
        runtimeNodeIds,
        visibleNodes,
        screenBounds,
        screenDistribution: summarizeScreenPointDistribution(screenPoints, width, height),
        edgeScreenDistribution: summarizeScreenEdgeDistribution(graph.getEdges?.() || [], nodePointById, width, height),
      };
    }

    function getGraphRuntimeTopologySnapshot() {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      if (!graph) {
        return {
          runtimeNodes: 0,
          runtimeEdges: 0,
          connectedNodeCount: 0,
          zeroDegreeNodeCount: 0,
          componentCount: 0,
          largestComponentNodeCount: 0,
          visibleZeroDegreeNodeCount: 0,
          zeroDegreeSample: [],
          components: [],
          edgeLength: null,
          routedEdgeCount: 0,
        };
      }
      const width = asNumber(graph.get?.("width"), 0);
      const height = asNumber(graph.get?.("height"), 0);
      const nodes = Array.isArray(graph.getNodes?.()) ? graph.getNodes() : [];
      const edges = Array.isArray(graph.getEdges?.()) ? graph.getEdges() : [];
      const nodeRows = [];
      const nodeById = new Map();
      nodes.forEach((item, index) => {
        const model = item?.getModel?.() || {};
        const id = text(model.id);
        if (!id) return;
        let point = null;
        const x = Number(model.x);
        const y = Number(model.y);
        if (Number.isFinite(x) && Number.isFinite(y) && typeof graph.getCanvasByPoint === "function") {
          try {
            const next = graph.getCanvasByPoint(x, y);
            if (next && Number.isFinite(next.x) && Number.isFinite(next.y)) point = next;
          } catch (_error) {}
        }
        const row = {
          id,
          index,
          model,
          degree: 0,
          neighbors: [],
          point,
          visible: !!point && point.x >= -80 && point.y >= -80 && point.x <= width + 80 && point.y <= height + 80,
        };
        nodeRows.push(row);
        nodeById.set(id, row);
      });
      const edgeLengths = [];
      let drawableEdges = 0;
      let routedEdgeCount = 0;
      edges.forEach((item) => {
        const model = item?.getModel?.() || {};
        const source = text(typeof model.source === "object" ? model.source?.id : model.source);
        const target = text(typeof model.target === "object" ? model.target?.id : model.target);
        const sourceRow = nodeById.get(source);
        const targetRow = nodeById.get(target);
        if (!sourceRow || !targetRow || sourceRow === targetRow) return;
        sourceRow.degree += 1;
        targetRow.degree += 1;
        sourceRow.neighbors.push(targetRow.index);
        targetRow.neighbors.push(sourceRow.index);
        if (asNumber(model.lineWidth, 0) > 0.05 && !model.layoutHiddenBySkeleton && text(model.layoutEdgeTier) !== "hidden") {
          drawableEdges += 1;
        }
        if (text(model.edgeRoute || model.route) || asNumber(model.edgeOffset, 0)) routedEdgeCount += 1;
        if (sourceRow.point && targetRow.point) {
          edgeLengths.push(Math.hypot(targetRow.point.x - sourceRow.point.x, targetRow.point.y - sourceRow.point.y));
        }
      });
      const visited = new Set();
      const components = [];
      nodeRows.forEach((row) => {
        if (visited.has(row.index)) return;
        const queue = [row.index];
        visited.add(row.index);
        const members = [];
        for (let cursor = 0; cursor < queue.length; cursor += 1) {
          const current = nodeRows[queue[cursor]];
          if (!current) continue;
          members.push(current);
          current.neighbors.forEach((nextIndex) => {
            if (visited.has(nextIndex)) return;
            visited.add(nextIndex);
            queue.push(nextIndex);
          });
        }
        const points = members.map((member) => member.point).filter(Boolean);
        components.push({
          size: members.length,
          zeroDegree: members.filter((member) => member.degree === 0).length,
          visible: members.filter((member) => member.visible).length,
          screenBounds: points.length
            ? {
                minX: Math.round(Math.min(...points.map((point) => point.x))),
                maxX: Math.round(Math.max(...points.map((point) => point.x))),
                minY: Math.round(Math.min(...points.map((point) => point.y))),
                maxY: Math.round(Math.max(...points.map((point) => point.y))),
              }
            : null,
          sample: members.slice(0, 8).map((member) => member.id),
        });
      });
      components.sort((left, right) => right.size - left.size || right.visible - left.visible);
      const zeroDegreeRows = nodeRows.filter((row) => row.degree === 0);
      const edgeLength = edgeLengths.length
        ? {
            count: edgeLengths.length,
            min: Math.round(Math.min(...edgeLengths) * 100) / 100,
            max: Math.round(Math.max(...edgeLengths) * 100) / 100,
            avg: Math.round((edgeLengths.reduce((sum, value) => sum + value, 0) / edgeLengths.length) * 100) / 100,
          }
        : null;
      const renderedEdgeGeometry = (() => {
        const data = graph?._edgeData;
        const segmentCount = Math.max(0, Number(graph?._edgeSegmentsCount || 0) || 0);
        if (!data || !segmentCount || typeof graph.getCanvasByPoint !== "function") return null;
        const points = [];
        const segments = [];
        const lengths = [];
        const stride = 12;
        for (let index = 0; index < segmentCount; index += 1) {
          const offset = index * stride;
          const sx = Number(data[offset]);
          const sy = Number(data[offset + 1]);
          const tx = Number(data[offset + 2]);
          const ty = Number(data[offset + 3]);
          if (![sx, sy, tx, ty].every(Number.isFinite)) continue;
          try {
            const start = graph.getCanvasByPoint(sx, sy);
            const end = graph.getCanvasByPoint(tx, ty);
            if (!start || !end || !Number.isFinite(start.x) || !Number.isFinite(start.y) || !Number.isFinite(end.x) || !Number.isFinite(end.y)) {
              continue;
            }
            points.push(start, end, { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 });
            segments.push({ start, end });
            lengths.push(Math.hypot(end.x - start.x, end.y - start.y));
          } catch (_error) {}
        }
        if (!points.length) return null;
        const bounds = {
          minX: Math.round(Math.min(...points.map((point) => point.x))),
          maxX: Math.round(Math.max(...points.map((point) => point.x))),
          minY: Math.round(Math.min(...points.map((point) => point.y))),
          maxY: Math.round(Math.max(...points.map((point) => point.y))),
        };
        const expanded = {
          minX: bounds.minX - 42,
          maxX: bounds.maxX + 42,
          minY: bounds.minY - 42,
          maxY: bounds.maxY + 42,
        };
        const visibleNodePoints = nodeRows.map((row) => row.point).filter(Boolean);
        const outsideNodes = visibleNodePoints.filter(
          (point) => point.x < expanded.minX || point.x > expanded.maxX || point.y < expanded.minY || point.y > expanded.maxY
        ).length;
        const distanceToSegment = (point, segment) => {
          const ax = Number(segment?.start?.x);
          const ay = Number(segment?.start?.y);
          const bx = Number(segment?.end?.x);
          const by = Number(segment?.end?.y);
          if (![ax, ay, bx, by, point?.x, point?.y].every(Number.isFinite)) return Infinity;
          const dx = bx - ax;
          const dy = by - ay;
          const lenSq = dx * dx + dy * dy;
          if (lenSq <= 0.0001) return Math.hypot(point.x - ax, point.y - ay);
          const t = Math.max(0, Math.min(1, ((point.x - ax) * dx + (point.y - ay) * dy) / lenSq));
          const px = ax + dx * t;
          const py = ay + dy * t;
          return Math.hypot(point.x - px, point.y - py);
        };
        const nodeEdgeDistances = nodeRows
          .filter((row) => row.visible && row.point)
          .map((row) => {
            let distance = Infinity;
            segments.forEach((segment) => {
              const next = distanceToSegment(row.point, segment);
              if (next < distance) distance = next;
            });
            return { row, distance };
          });
        const farNodeThreshold = 46;
        const farNodeRows = nodeEdgeDistances.filter((entry) => !Number.isFinite(entry.distance) || entry.distance > farNodeThreshold);
        const sortedDistances = nodeEdgeDistances
          .map((entry) => entry.distance)
          .filter(Number.isFinite)
          .sort((left, right) => left - right);
        const p90Index = Math.max(0, Math.min(sortedDistances.length - 1, Math.floor(sortedDistances.length * 0.9)));
        const spanX = Math.max(0, bounds.maxX - bounds.minX);
        const spanY = Math.max(0, bounds.maxY - bounds.minY);
        return {
          segmentCount,
          sampledPointCount: points.length,
          screenBounds: bounds,
          bboxAreaRatio: width > 0 && height > 0 ? (spanX * spanY) / Math.max(1, width * height) : 0,
          horizontalSpreadRatio: width > 0 ? spanX / width : 0,
          verticalSpreadRatio: height > 0 ? spanY / height : 0,
          nodeOutsideRenderedEdgeBoundsRatio: outsideNodes / Math.max(1, visibleNodePoints.length),
          nodeFarFromRenderedEdgeRatio: farNodeRows.length / Math.max(1, nodeEdgeDistances.length),
          nodeEdgeDistanceP90: sortedDistances.length ? Math.round(sortedDistances[p90Index] * 100) / 100 : 0,
          farNodeSample: farNodeRows.slice(0, 12).map((entry) => ({
            id: entry.row.id,
            degree: entry.row.degree,
            x: entry.row.point ? Math.round(entry.row.point.x) : null,
            y: entry.row.point ? Math.round(entry.row.point.y) : null,
            distance: Number.isFinite(entry.distance) ? Math.round(entry.distance * 100) / 100 : null,
          })),
          maxSegmentLength: lengths.length ? Math.round(Math.max(...lengths) * 100) / 100 : 0,
          avgSegmentLength: lengths.length
            ? Math.round((lengths.reduce((sum, value) => sum + value, 0) / lengths.length) * 100) / 100
            : 0,
        };
      })();
      return {
        runtimeNodes: nodeRows.length,
        runtimeEdges: edges.length,
        runtimeBufferCounts: {
          nodeCount: Math.max(0, Number(graph?._nodeCount || 0) || 0),
          edgeSegmentsCount: Math.max(0, Number(graph?._edgeSegmentsCount || 0) || 0),
          shadowCount: Math.max(0, Number(graph?._shadowCount || 0) || 0),
          nodeBufferInstances: graph?._nodeData ? Math.floor((graph._nodeData.length || 0) / 15) : 0,
          edgeBufferSegments: graph?._edgeData ? Math.floor((graph._edgeData.length || 0) / 12) : 0,
        },
        connectedNodeCount: nodeRows.length - zeroDegreeRows.length,
        zeroDegreeNodeCount: zeroDegreeRows.length,
        componentCount: components.length,
        largestComponentNodeCount: components[0]?.size || 0,
        visibleZeroDegreeNodeCount: zeroDegreeRows.filter((row) => row.visible).length,
        zeroDegreeSample: zeroDegreeRows.slice(0, 16).map((row) => row.id),
        components: components.slice(0, 8),
        peripheralNodeSample: nodeRows
          .slice()
          .sort((left, right) => {
            const leftPoint = left.point || { x: 0, y: 0 };
            const rightPoint = right.point || { x: 0, y: 0 };
            return rightPoint.y - leftPoint.y || rightPoint.x - leftPoint.x || left.id.localeCompare(right.id, "zh-CN");
          })
          .slice(0, 18)
          .map((row) => ({
            id: row.id,
            degree: row.degree,
            x: row.point ? Math.round(row.point.x) : null,
            y: row.point ? Math.round(row.point.y) : null,
            neighbors: row.neighbors
              .slice(0, 6)
              .map((index) => nodeRows[index]?.id || "")
              .filter(Boolean),
          })),
        drawableEdges,
        edgeLength,
        renderedEdgeGeometry,
        routedEdgeCount,
      };
    }

    function getTextAtlasSnapshot() {
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      if (!graph || typeof graph.getTextAtlasStats !== "function") {
        const emptySnapshot = {
          setCount: 0,
          atlasCount: 0,
          glyphCount: 0,
          approxTextureBytes: 0,
          batchCount: 0,
          byKey: [],
          traceId: text(state?.perfMetrics?.layoutWorker?.lastTraceId),
        };
        if (typeof recordTextAtlasMetrics === "function") {
          recordTextAtlasMetrics(emptySnapshot);
        }
        return emptySnapshot;
      }
      const snapshot = {
        ...(graph.getTextAtlasStats() || {}),
        traceId: text(state?.perfMetrics?.layoutWorker?.lastTraceId),
      };
      if (typeof recordTextAtlasMetrics === "function") {
        recordTextAtlasMetrics(snapshot);
      }
      return snapshot;
    }

    function getPerfSnapshot() {
      const textAtlas = getTextAtlasSnapshot();
      const graphPatch =
        state?.graphMutationStore && typeof state.graphMutationStore.getMetricsSnapshot === "function"
          ? state.graphMutationStore.getMetricsSnapshot()
          : {};
      const backendPerf =
        state?.backend && typeof state.backend.getFlowPerfSnapshot === "function" ? state.backend.getFlowPerfSnapshot() : {};
      const graphPatchBridge =
        backendPerf && typeof backendPerf === "object" && backendPerf.graphPatch && typeof backendPerf.graphPatch === "object"
          ? backendPerf.graphPatch
          : {};
      const build =
        backendPerf && typeof backendPerf === "object" && backendPerf.build && typeof backendPerf.build === "object"
          ? backendPerf.build
          : {};
      const transport =
        backendPerf && typeof backendPerf === "object" && backendPerf.transport && typeof backendPerf.transport === "object"
          ? backendPerf.transport
          : {};
      const runtimeBridge =
        backendPerf && typeof backendPerf === "object" && backendPerf.runtimeBridge && typeof backendPerf.runtimeBridge === "object"
          ? backendPerf.runtimeBridge
          : {};
      const graphCommands =
        state?.graphMutationCommands && typeof state.graphMutationCommands.getMetricsSnapshot === "function"
          ? state.graphMutationCommands.getMetricsSnapshot()
          : {};
      const canvasInteraction =
        state?.canvasInteractionStore && typeof state.canvasInteractionStore.getMetricsSnapshot === "function"
          ? state.canvasInteractionStore.getMetricsSnapshot()
          : {};
      const networkSectorPlacement =
        typeof getNetworkSectorPlacementCacheSnapshot === "function"
          ? getNetworkSectorPlacementCacheSnapshot()
          : {};
      if (flowObservabilityAdapter && typeof flowObservabilityAdapter.buildFlowPerfSnapshot === "function") {
        return {
          ...flowObservabilityAdapter.buildFlowPerfSnapshot(state?.perfMetrics, {
            graphPatch,
            graphPatchBridge,
            build,
            transport,
            runtimeBridge,
            canvasInteraction,
          }),
          graphCommands,
          networkSectorPlacement,
        };
      }
      return {
        graphPatch,
        graphPatchBridge,
        build,
        transport,
        runtimeBridge,
        graphCommands,
        canvasInteraction,
        networkSectorPlacement,
        layoutWorker: state?.perfMetrics?.layoutWorker || {},
        textAtlas: state?.perfMetrics?.textAtlas || {},
        windows: {},
        traces: {},
        timestamp: Date.now(),
      };
    }

    function exportPerfSnapshot() {
      const snapshot = getPerfSnapshot();
      if (flowObservabilityAdapter && typeof flowObservabilityAdapter.exportFlowPerfSnapshot === "function") {
        return flowObservabilityAdapter.exportFlowPerfSnapshot(snapshot);
      }
      return JSON.stringify({ exportedAt: Date.now(), snapshot }, null, 2);
    }

    return {
      getLayoutSnapshot,
      getRendererSnapshot,
      getGraphSelectionSnapshot,
      getGraphItemStyleSnapshot,
      getGraphEdgePresentationSnapshot,
      triggerNetworkDenseHoverRevealForDebug,
      restoreNetworkDenseHoverRevealForDebug,
      getGraphViewportSnapshot,
      getGraphRuntimeTopologySnapshot,
      getTextAtlasSnapshot,
      getPerfSnapshot,
      exportPerfSnapshot,
    };
  }

  window.__ANALYTIX_FLOW_DEBUG_SURFACE__ = {
    createFlowDebugSurface,
  };
})();
