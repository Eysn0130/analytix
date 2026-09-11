(() => {
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__ || null;
  if (
    !piiProjection ||
    typeof piiProjection.projectField !== "function" ||
    typeof piiProjection.resolveKind !== "function"
  ) {
    throw new Error("ordinary PII projection missing for canvas drill controller");
  }

  const LOG_ACCOUNT_FIELDS = new Set(["accountKey", "from", "to"]);
  const LOG_IDENTIFIER_FIELDS = new Set(["base", "id", "leftId", "mapped", "nodeId", "nodeTitle", "source", "target", "title"]);

  function projectLogPayload(value, fieldName = "", ancestors = new WeakSet()) {
    if (typeof value === "string" || typeof value === "number") {
      if (
        typeof value === "number" &&
        !LOG_ACCOUNT_FIELDS.has(fieldName) &&
        !LOG_IDENTIFIER_FIELDS.has(fieldName) &&
        !piiProjection.resolveKind(fieldName)
      ) {
        return value;
      }
      const projectionField = LOG_ACCOUNT_FIELDS.has(fieldName)
        ? "account_no"
        : LOG_IDENTIFIER_FIELDS.has(fieldName)
          ? "node_id"
          : fieldName;
      return piiProjection.projectField(projectionField, value);
    }
    if (!value || typeof value !== "object") return value;
    if (ancestors.has(value)) return "[Circular]";
    ancestors.add(value);
    const projected = Array.isArray(value)
      ? value.map((item) => projectLogPayload(item, fieldName, ancestors))
      : Object.fromEntries(
          Object.entries(value).map(([key, item]) => [key, projectLogPayload(item, key, ancestors)])
        );
    ancestors.delete(value);
    return projected;
  }

  function safeCall(fn, ...args) {
    if (typeof fn !== "function") return undefined;
    try {
      return fn(...args);
    } catch (e) {
      return undefined;
    }
  }

  function safeLog(log, level, event, payload) {
    return safeCall(log, level, event, projectLogPayload(payload));
  }

  async function safeAwait(fn, ...args) {
    if (typeof fn !== "function") return undefined;
    return await fn(...args);
  }

  function createCanvasDrillController({
    state = null,
    txnDrillState = null,
    ensureGraph = null,
    captureNodePositions = null,
    layoutScale = null,
    applyEdgeOffsets = null,
    replaceRuntimeGraphData = null,
    applyStyleToGraph = null,
    applyAnalysisEncodings = null,
    applyEdgeDetailLabels = null,
    syncEdgeDetailButton = null,
    animateToLayout = null,
    syncGraphNodePositions = null,
    updateGraphStats = null,
    updateCurrentViewSnapshot = null,
    focusGraphItemById = null,
    scheduleGraphPatch = null,
    parseTxnTimeValue = null,
    fetchAccountTxnRows = null,
    normalizeAccountTxnRows = null,
    computeDrillOutflows = null,
    buildTxnNodeId = null,
    buildTxnNodeModel = null,
    buildTxnEdgeModel = null,
    updateGraphDataById = null,
    getGraphNodeInfo = null,
    getGraphExtremes = null,
    log = null,
    toast = null,
    getDrillConfig = null,
  } = {}) {
    async function mergeDrillGraph(extraNodes, extraEdges, { focusId = "", focusPos } = {}) {
      const graph = safeCall(ensureGraph);
      if (!graph) return;
      const prevPositions = safeCall(captureNodePositions, graph) || new Map();
      const liveNodes = (graph.getNodes?.() || []).map((n) => ({ ...(n.getModel?.() || {}) }));
      const liveEdges = (graph.getEdges?.() || []).map((e) => ({ ...(e.getModel?.() || {}) }));
      const nodeMap = new Map(liveNodes.map((n) => [n.id, n]));
      const edgeMap = new Map(liveEdges.map((e) => [e.id, e]));
      let anchor = null;
      if (focusPos && Number.isFinite(focusPos.x) && Number.isFinite(focusPos.y)) {
        anchor = { x: focusPos.x, y: focusPos.y };
      } else if (focusId && nodeMap.has(focusId)) {
        const model = nodeMap.get(focusId);
        if (Number.isFinite(model?.x) && Number.isFinite(model?.y)) {
          anchor = { x: model.x, y: model.y };
        }
      }
      if (!anchor) anchor = { x: 0, y: 0 };
      const totalCount = liveNodes.length + (extraNodes?.length || 0);
      const scale = Number(safeCall(layoutScale, totalCount) || 1);
      const zoom = graph.getZoom?.() || 1;
      const baseDistRaw = 190 * scale + Math.min(90, (extraNodes?.length || 0) * 18);
      const baseDist = baseDistRaw / Math.max(0.8, zoom);
      const spread = Math.max(90 * scale, 70);
      const newNodes = Array.isArray(extraNodes) ? extraNodes.slice() : [];
      newNodes.sort((a, b) => String(a.title || a.name || a.id).localeCompare(String(b.title || b.name || b.id), "zh-CN"));
      const mid = (newNodes.length - 1) / 2;
      const targets = new Map();
      const startAddNodes = [];
      newNodes.forEach((n, idx) => {
        if (!n || !n.id) return;
        const offset = (idx - mid) * spread;
        const x = anchor.x + baseDist;
        const y = anchor.y + offset;
        if (!nodeMap.has(n.id)) {
          nodeMap.set(n.id, { ...n, x, y });
          targets.set(n.id, { x, y });
          startAddNodes.push({ ...n, x: anchor.x, y: anchor.y });
        } else if (!targets.has(n.id)) {
          targets.set(n.id, { x, y });
        }
      });
      (extraEdges || []).forEach((edge) => {
        if (!edge || !edge.id) return;
        if (!edgeMap.has(edge.id)) edgeMap.set(edge.id, edge);
      });
      const nodes = Array.from(nodeMap.values());
      const edges = Array.from(edgeMap.values());
      await safeAwait(applyEdgeOffsets, edges, { reason: "merge-drill-graph" });
      safeCall(replaceRuntimeGraphData, { nodes, edges }, { reason: "merge-drill-graph" });
      if (startAddNodes.length) {
        graph.setAutoPaint(false);
        try {
          startAddNodes.forEach((node) => graph.addItem("node", node));
          (extraEdges || []).forEach((edge) => graph.addItem("edge", edge));
          const edgeById = new Map(edges.map((edge) => [edge.id, edge]));
          graph.getEdges?.().forEach((edge) => {
            const model = edge.getModel?.() || {};
            const updated = edgeById.get(model.id);
            if (!updated) return;
            if (updated.edgeOffset !== model.edgeOffset) {
              graph.updateItem(edge, { edgeOffset: updated.edgeOffset });
            }
          });
        } catch (e) {}
        graph.setAutoPaint(true);
        graph.paint?.();
      } else {
        try {
          graph.changeData(state?.graph?.data);
          graph.getEdges?.().forEach((edge) => graph.refreshItem?.(edge));
          graph.paint?.();
        } catch (e) {}
      }
      safeCall(applyStyleToGraph, state?.graphStyle, { syncView: false });
      safeCall(applyAnalysisEncodings);
      if (state?.edgeDetail) safeCall(applyEdgeDetailLabels, true);
      safeCall(syncEdgeDetailButton);
      if (targets.size) {
        safeCall(animateToLayout, null, targets, {
          graph,
          duration: 520,
          finalize: false,
          fit: false,
          pinMap: prevPositions,
          onComplete: () => {
            safeCall(syncGraphNodePositions, graph);
            safeCall(updateGraphStats);
            safeCall(updateCurrentViewSnapshot);
            safeCall(focusGraphItemById, focusId, { animate: false, pulse: true });
            safeCall(scheduleGraphPatch, {
              scope: "local-update",
              reason: "merge-drill-graph",
            });
          },
        });
      } else {
        safeCall(updateGraphStats);
        safeCall(updateCurrentViewSnapshot);
        safeCall(focusGraphItemById, focusId, { animate: false, pulse: true });
        safeCall(scheduleGraphPatch, {
          scope: "local-update",
          reason: "merge-drill-graph",
        });
      }
    }

    async function drillTxnNode(model, ctxPos = null) {
      if (!model || !model.drill || !model.drill.accountKey) {
        safeCall(toast, "暂无可穿透数据", "warn");
        return;
      }
      if (model.drillExpanded) {
        safeCall(toast, "已穿透", "warn");
        return;
      }
      if (txnDrillState?.loading) return;
      if (!state?.backend || typeof state.backend.getAccountTxnRows !== "function") {
        safeCall(toast, "后端未就绪", "warn");
        return;
      }

      txnDrillState.loading = true;
      const drill = model.drill;
      const drillMutationSeq = Number(state?.graphMutationSeq || 0);
      try {
        const baseTxn = drill?.baseTxn || {};
        safeLog(log, "INFO", "drill start", {
          nodeId: model.id,
          nodeTitle: model.title || model.label || "",
          accountKey: drill?.accountKey || "",
          dateStart: drill?.dateStart || "",
          dateEnd: drill?.dateEnd || "",
          policy: safeCall(getDrillConfig)?.policy,
          windowDays: safeCall(getDrillConfig)?.windowDays,
          baseTxn: {
            amount: baseTxn.amount ?? null,
            time: baseTxn.time || "",
            txn_id: baseTxn.txn_id || "",
            from: baseTxn?.from?.account || "",
            to: baseTxn?.to?.account || "",
          },
        });
      } catch (e) {}
      try {
        const cacheKey = [
          state?.caseId || "",
          drill.accountKey || "",
          drill?.baseTxn?.time || "",
          drill?.dateStart || "",
          drill?.dateEnd || "",
        ].join("|");
        let rows = txnDrillState?.cache?.get(cacheKey);
        const cacheHit = !!rows;
        if (!rows) {
          const baseTime = drill?.baseTxn?.time || "";
          const startTime = safeCall(parseTxnTimeValue, baseTime) != null ? baseTime : "";
          rows = await safeCall(fetchAccountTxnRows, drill.accountKey, {
            dateStart: drill?.dateStart || "",
            dateEnd: drill?.dateEnd || "",
            startTime,
            sortDir: "asc",
          });
          txnDrillState?.cache?.set(cacheKey, rows);
        }
        safeLog(log, "INFO", "drill rows fetched", {
          nodeId: model.id,
          accountKey: drill?.accountKey || "",
          rows: rows?.length || 0,
          cacheHit,
        });
        const normalized = safeCall(normalizeAccountTxnRows, rows || []) || [];
        const {
          inbound,
          outflows,
          baseBalance,
          anchorTime,
          policy,
          windowDays,
          maxTime,
        } = safeCall(computeDrillOutflows, normalized, drill.baseTxn || {}) || {};
        safeLog(log, "INFO", "drill match summary", {
          nodeId: model.id,
          policy,
          windowDays,
          inbound: inbound
            ? {
                txn_time: inbound.txn_time || "",
                amount: inbound.amount ?? null,
                balance: inbound.balance ?? null,
                counterparty_acct: inbound.counterparty_acct || "",
                counterparty_name: inbound.counterparty_name || "",
                txn_id: inbound.txn_id || inbound.__txid || "",
                matchReason: inbound.__matchReason || "",
              }
            : null,
          outflows: outflows?.length || 0,
          outflowSample: (outflows || []).slice(0, 3).map((row) => ({
            txn_time: row.txn_time || "",
            amount: row.amount ?? null,
            counterparty_acct: row.counterparty_acct || "",
            counterparty_name: row.counterparty_name || "",
            txn_id: row.txn_id || row.__txid || "",
          })),
          baseBalance: baseBalance ?? null,
          anchorTime: anchorTime ?? null,
          maxTime: maxTime ?? null,
        });
        if (state?.debugDrill) {
          try {
            const baseTxn = drill?.baseTxn || {};
            const limit = 12;
            const sample = (outflows || []).slice(0, limit).map((row) => {
              const t = safeCall(parseTxnTimeValue, row.txn_time);
              const afterAnchor = anchorTime != null ? t != null && t >= anchorTime : true;
              const balanceRaw = row.balance;
              const bal = balanceRaw == null || (typeof balanceRaw === "string" && !balanceRaw.trim())
                ? Number.NaN
                : Number(balanceRaw);
              const stopAfter =
                baseBalance != null && Number.isFinite(bal) ? bal < baseBalance - 0.0001 : false;
              return {
                txn_time: row.txn_time || "",
                amount: row.amount ?? null,
                balance: Number.isFinite(bal) ? bal : null,
                counterparty_acct: row.counterparty_acct || "",
                counterparty_name: row.counterparty_name || "",
                reason: { afterAnchor, stopAfter },
              };
            });
            safeLog(log, "INFO", "drill outflow explain", {
              nodeId: model.id,
              accountKey: drill?.accountKey || "",
              policy,
              windowDays,
              baseTxn: {
                amount: baseTxn.amount ?? null,
                time: baseTxn.time || "",
                txn_id: baseTxn.txn_id || "",
                from: baseTxn?.from?.account || "",
                to: baseTxn?.to?.account || "",
              },
              inboundMatch: inbound?.__matchReason || "",
              rules: {
                anchorTime: anchorTime ?? null,
                timeRule: anchorTime != null ? "txn_time >= anchorTime" : "no_anchor_time",
                timeWindow: maxTime != null ? `<= ${new Date(maxTime).toISOString()}` : "no_time_window",
                balanceRule: baseBalance != null ? `stop when balance < ${baseBalance.toFixed(2)}` : "no_balance_rule",
              },
              total: outflows?.length || 0,
              sample,
              truncated: (outflows || []).length > limit,
            });
          } catch (e) {}
        }

        if (!(outflows || []).length) {
          safeCall(toast, inbound ? "未找到可穿透转出" : "未匹配到入账记录", "warn");
          return;
        }
        if (Number(state?.graphMutationSeq || 0) !== drillMutationSeq) {
          safeLog(log, "INFO", "drill aborted by topology mutation", {
            nodeId: model.id || "",
            startSeq: drillMutationSeq,
            currentSeq: state?.graphMutationSeq || 0,
          });
          return;
        }

        const graph = safeCall(ensureGraph);
        const newNodes = [];
        const newEdges = [];
        const edgeIds = new Set();
        const existingNodeIds = new Set();
        const existingEdgeIds = new Set();
        const drillNodeAlias = new Map();
        const remappedNodes = [];
        const remappedBase = new Set();
        const usedNodeIds = new Set();
        const addedNodeIds = new Set();
        try {
          graph?.getNodes?.().forEach((node) => {
            const id = node.getModel?.()?.id ?? "";
            if (id) {
              existingNodeIds.add(id);
              usedNodeIds.add(id);
            }
          });
          graph?.getEdges?.().forEach((edge) => {
            const id = edge.getModel?.()?.id ?? "";
            if (id) existingEdgeIds.add(id);
          });
          existingEdgeIds.forEach((id) => edgeIds.add(id));
        } catch (e) {}

        const ensureUniqueId = (base, used, fallbackPrefix) => {
          let id = String(base || "").trim();
          if (!id) id = `${fallbackPrefix || "id"}-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
          if (!used.has(id)) {
            used.add(id);
            return id;
          }
          let idx = 1;
          let next = `${id}-${idx}`;
          while (used.has(next) && idx < 1000) {
            idx += 1;
            next = `${id}-${idx}`;
          }
          used.add(next);
          return next;
        };

        const ensureUniqueNodeId = (base) => {
          const raw = String(base || "").trim();
          if (!raw) {
            return ensureUniqueId("", usedNodeIds, "drill-node");
          }
          if (drillNodeAlias.has(raw)) return drillNodeAlias.get(raw);
          let id = raw;
          if (existingNodeIds.has(id) || usedNodeIds.has(id)) {
            let idx = 1;
            let next = `${id}__drill${idx}`;
            while ((existingNodeIds.has(next) || usedNodeIds.has(next)) && idx < 1000) {
              idx += 1;
              next = `${id}__drill${idx}`;
            }
            id = next;
          }
          existingNodeIds.add(id);
          usedNodeIds.add(id);
          drillNodeAlias.set(raw, id);
          if (id !== raw && !remappedBase.has(raw)) {
            remappedBase.add(raw);
            remappedNodes.push({ base: raw, mapped: id });
          }
          return id;
        };

        (outflows || []).forEach((row) => {
          const acct = String(row.counterparty_acct || "").trim();
          const name = String(row.counterparty_name || "").trim();
          const baseId = safeCall(buildTxnNodeId, acct, name, row.__txid ? `drill-${row.__txid}` : "");
          const nodeId = ensureUniqueNodeId(baseId);
          if (!addedNodeIds.has(nodeId)) {
            addedNodeIds.add(nodeId);
            newNodes.push(safeCall(buildTxnNodeModel, { account: acct, name, id: nodeId, drillable: false }));
          }
          const edgeBase = row.__txid ? `drill-${model.id}-${row.__txid}` : `${model.id}=>${nodeId}@${row.txn_time || ""}`;
          const edgeId = ensureUniqueId(edgeBase, edgeIds, "drill-edge");
          newEdges.push(
            safeCall(buildTxnEdgeModel, {
              id: edgeId,
              source: model.id,
              target: nodeId,
              amount: row.amount,
              time: row.txn_time || "",
            })
          );
        });

        if (remappedNodes.length) {
          safeLog(log, "INFO", "drill node id remap", {
            nodeId: model.id,
            count: remappedNodes.length,
            sample: remappedNodes.slice(0, 4),
          });
        }

        safeLog(log, "INFO", "drill graph plan", {
          nodeId: model.id,
          newNodes: newNodes.length || 0,
          newEdges: newEdges.length || 0,
          edgeSample: newEdges.slice(0, 4).map((edge) => ({
            id: edge.id || "",
            source: edge.source || "",
            target: edge.target || "",
            label: edge.label || "",
          })),
        });

        const item = graph?.findById?.(model.id);
        if (item) {
          graph.updateItem(item, { drillExpanded: true });
          safeCall(updateGraphDataById, [item], { drillExpanded: true }, "nodes");
        } else {
          const dataNode = (state?.graph?.data?.nodes || []).find((node) => node.id === model.id);
          if (dataNode) dataNode.drillExpanded = true;
        }

        let focusPos = null;
        let focusPosSource = "";
        let focusPosDelta = null;
        try {
          const itemModel = item?.getModel?.();
          if (itemModel && Number.isFinite(itemModel.x) && Number.isFinite(itemModel.y)) {
            focusPos = { x: itemModel.x, y: itemModel.y };
            focusPosSource = "model";
          }
        } catch (e) {}
        if (ctxPos && ctxPos.id === model.id && Number.isFinite(ctxPos.graphX) && Number.isFinite(ctxPos.graphY)) {
          const ctxPoint = { x: ctxPos.graphX, y: ctxPos.graphY };
          if (focusPos && Number.isFinite(focusPos.x) && Number.isFinite(focusPos.y)) {
            focusPosDelta = Math.round(Math.hypot(ctxPoint.x - focusPos.x, ctxPoint.y - focusPos.y) * 100) / 100;
            if (focusPosDelta <= 12) {
              focusPos = ctxPoint;
              focusPosSource = "ctx";
            }
          } else {
            focusPos = ctxPoint;
            focusPosSource = "ctx";
          }
        }
        if (state?.debugLayout) {
          safeLog(log, "INFO", "drill anchor pick", {
            nodeId: model.id,
            focusPos: focusPos ? { x: Math.round(focusPos.x * 100) / 100, y: Math.round(focusPos.y * 100) / 100 } : null,
            source: focusPosSource,
            ctxDelta: focusPosDelta,
          });
        }

        if (Number(state?.graphMutationSeq || 0) !== drillMutationSeq) {
          safeLog(log, "INFO", "drill merge skipped by topology mutation", {
            nodeId: model.id || "",
            startSeq: drillMutationSeq,
            currentSeq: state?.graphMutationSeq || 0,
          });
          return;
        }

        await mergeDrillGraph(newNodes, newEdges, { focusId: model.id, focusPos });

        if (state?.debugLayout) {
          setTimeout(() => {
            try {
              const g = safeCall(ensureGraph);
              if (!g) return;
              const focusInfo = safeCall(getGraphNodeInfo, g, model.id);
              const newInfos = newNodes.map((node) => safeCall(getGraphNodeInfo, g, node.id)).filter(Boolean);
              const sample = newInfos.slice(0, 4);
              const extremes = safeCall(getGraphExtremes, g);
              const leftId = extremes?.left?.id || "";
              let minDx = null;
              let maxDx = null;
              let minNewX = null;
              let maxNewX = null;
              const focusX = Number.isFinite(focusInfo?.x) ? focusInfo.x : null;
              if (focusX != null) {
                newInfos.forEach((info) => {
                  if (!info || !Number.isFinite(info.x)) return;
                  const dx = info.x - focusX;
                  minDx = minDx == null ? dx : Math.min(minDx, dx);
                  maxDx = maxDx == null ? dx : Math.max(maxDx, dx);
                  minNewX = minNewX == null ? info.x : Math.min(minNewX, info.x);
                  maxNewX = maxNewX == null ? info.x : Math.max(maxNewX, info.x);
                });
              }
              const edgeSample = newEdges.slice(0, 4).map((edge) => {
                const edgeItem = g.findById?.(edge.id);
                const modelEdge = edgeItem?.getModel?.() || edge || {};
                return {
                  id: modelEdge.id || "",
                  source: modelEdge.source || "",
                  target: modelEdge.target || "",
                  sourcePos: safeCall(getGraphNodeInfo, g, modelEdge.source),
                  targetPos: safeCall(getGraphNodeInfo, g, modelEdge.target),
                };
              });
              const wrongSource = newEdges.filter((edge) => edge.source !== model.id);
              const touchLeft = leftId ? newEdges.filter((edge) => edge.source === leftId || edge.target === leftId) : [];
              safeLog(log, "INFO", "drill position check", {
                nodeId: model.id,
                focus: focusInfo || null,
                sample,
                extremes,
                zoom: g.getZoom?.() ?? null,
                introAnimating: !!state?.introAnimating,
              });
              safeLog(log, "INFO", "drill layout verdict", {
                nodeId: model.id,
                focusX: focusX ?? null,
                newCount: newInfos.length,
                minDx: Number.isFinite(minDx) ? Math.round(minDx * 100) / 100 : null,
                maxDx: Number.isFinite(maxDx) ? Math.round(maxDx * 100) / 100 : null,
                minNewX: Number.isFinite(minNewX) ? Math.round(minNewX * 100) / 100 : null,
                maxNewX: Number.isFinite(maxNewX) ? Math.round(maxNewX * 100) / 100 : null,
                anyLeftOfFocus: minDx != null ? minDx < -4 : null,
              });
              safeLog(log, "INFO", "drill edge check", {
                nodeId: model.id,
                leftId,
                wrongSourceCount: wrongSource.length,
                touchLeftCount: touchLeft.length,
                edgeSample,
              });
            } catch (e) {}
          }, 680);
          safeLog(log, "INFO", "drill merge done", {
            nodeId: model.id,
            newNodes: newNodes.length || 0,
            newEdges: newEdges.length || 0,
            totals: {
              nodes: state?.graph?.data?.nodes?.length || 0,
              edges: state?.graph?.data?.edges?.length || 0,
            },
          });
        }

        const summary = `${newNodes.length} 节点 / ${newEdges.length} 边`;
        const hint = (() => {
          if (policy === "time") return windowDays > 0 ? `时间窗${windowDays}天` : "时间窗不限";
          if (policy === "balance") return windowDays > 0 ? `余额回落·${windowDays}天` : "余额回落";
          if (baseBalance == null) return windowDays > 0 ? `时间窗${windowDays}天` : "时间窗不限";
          return windowDays > 0 ? `余额回落·${windowDays}天` : "余额回落";
        })();
        safeCall(toast, `穿透完成 · ${summary} · ${hint}`, "ok");
      } catch (e) {
        safeLog(log, "WARN", "drill failed", {
          nodeId: model.id,
          message: String(e?.message || e),
          stack: e?.stack || "",
        });
        safeCall(toast, "穿透失败", "danger");
      } finally {
        if (txnDrillState) txnDrillState.loading = false;
      }
    }

    return {
      mergeDrillGraph,
      drillTxnNode,
    };
  }

  window.__ANALYTIX_FLOW_CANVAS_DRILL_CONTROLLER__ = {
    createCanvasDrillController,
    projectLogPayload,
  };
})();
