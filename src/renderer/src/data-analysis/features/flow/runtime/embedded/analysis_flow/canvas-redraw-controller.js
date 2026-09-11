(() => {
  function safeCall(fn, ...args) {
    if (typeof fn !== "function") return undefined;
    try {
      return fn(...args);
    } catch (e) {
      return undefined;
    }
  }

  function safeCounts(value) {
    const next = value && typeof value === "object" ? value : {};
    return {
      expected: {
        nodes: Number(next?.expected?.nodes || 0),
        edges: Number(next?.expected?.edges || 0),
      },
      runtime: {
        nodes: Number(next?.runtime?.nodes || 0),
        edges: Number(next?.runtime?.edges || 0),
      },
      delta: {
        nodes: Number(next?.delta?.nodes || 0),
        edges: Number(next?.delta?.edges || 0),
      },
    };
  }

  function createCanvasRedrawController({
    state = null,
    ensureGraph = null,
    isDeleteReason = null,
    isDeleteProbeVerbose = null,
    shouldLogConsistencyProbeNormal = null,
    shouldAlwaysRecreateAfterDelete = null,
    shouldRunDeleteVisualGuard = null,
    graphEdgeIdentityKey = null,
    collectGhostProbeCounts = null,
    isGraphRuntimeStable = null,
    cancelGraphTransientTasks = null,
    logGhostProbe = null,
    log = null,
    captureGraphViewport = null,
    applyGraphViewport = null,
    ensureGraphDataVisible = null,
    cloneGraphData = null,
    runCanvasRedraw = null,
    updateMinimap = null,
    applyStyleToGraph = null,
    applyAnalysisEncodings = null,
    applyEdgeDetailLabels = null,
    syncEdgeDetailButton = null,
    destroyMinimap = null,
    beforeGraphRecreate = null,
  } = {}) {
    function prepareGraphRedrawMask(graph, reason = "", enabled = false) {
      if (!enabled || !graph) return null;
      const renderer = String(graph.getRenderer?.() || graph.get?.("renderer") || "").toLowerCase();
      if (renderer !== "webgl") return null;
      const container = graph.get?.("container");
      if (!container || typeof container.querySelectorAll !== "function") return null;
      const layers = Array.from(container.querySelectorAll("canvas.graphWebglCanvas")).filter(Boolean);
      if (!layers.length) return null;
      const snapshot = layers.map((el) => ({
        el,
        opacity: el.style.opacity || "",
        visibility: el.style.visibility || "",
        willChange: el.style.willChange || "",
      }));
      snapshot.forEach((row) => {
        const el = row.el;
        el.style.willChange = "opacity";
        el.style.visibility = "visible";
        el.style.opacity = "0";
      });
      if (!safeCall(isDeleteReason, reason) || safeCall(isDeleteProbeVerbose)) {
        safeCall(
          logGhostProbe,
          "force-redraw-mask",
          {
            reason: reason || "force-redraw",
            enabled: true,
            layers: snapshot.length,
            renderer,
          },
          {
            dedupKey: `force-redraw-mask|${state?.graphMutationSeq || 0}|${reason || ""}|on`,
            dedupMs: 120,
            onlyInDeleteWindow: true,
          }
        );
      }
      let restored = false;
      const restore = () => {
        if (restored) return;
        restored = true;
        snapshot.forEach((row) => {
          try {
            row.el.style.opacity = row.opacity;
            row.el.style.visibility = row.visibility;
            row.el.style.willChange = row.willChange;
          } catch (e) {}
        });
        requestAnimationFrame(() => {
          try {
            graph.paint?.();
          } catch (e) {}
        });
        if (safeCall(isDeleteProbeVerbose)) {
          safeCall(
            logGhostProbe,
            "force-redraw-mask",
            {
              reason: reason || "force-redraw",
              enabled: false,
              layers: snapshot.length,
              renderer,
            },
            {
              dedupKey: `force-redraw-mask|${state?.graphMutationSeq || 0}|${reason || ""}|off`,
              dedupMs: 120,
              onlyInDeleteWindow: true,
            }
          );
        }
      };
      return restore;
    }

    function forceRedrawGraphFromState(reason = "", { restoreViewport = true, hideDuringRedraw = false } = {}) {
      const graph = safeCall(ensureGraph);
      if (!graph) return false;
      const data = state?.graph?.data || { nodes: [], edges: [] };
      const nodes = Array.isArray(data.nodes) ? data.nodes : [];
      const edges = Array.isArray(data.edges) ? data.edges : [];
      const viewport = restoreViewport ? safeCall(captureGraphViewport) : null;
      const restoreMask = prepareGraphRedrawMask(graph, reason, !!hideDuringRedraw);
      const countsBefore = safeCounts(safeCall(collectGhostProbeCounts, graph));
      const logNormal = !safeCall(isDeleteReason, reason) || safeCall(isDeleteProbeVerbose);
      if (logNormal) {
        safeCall(logGhostProbe, "force-redraw-start", {
          reason: reason || "force-redraw",
          restoreViewport: !!restoreViewport,
          hideDuringRedraw: !!hideDuringRedraw,
          before: countsBefore,
        });
      }
      return safeCall(runCanvasRedraw, reason || "force-redraw", () => {
        try {
          const payload =
            typeof cloneGraphData === "function" ? cloneGraphData({ nodes, edges }) : { nodes: [...nodes], edges: [...edges] };
          graph.clear?.();
          graph.data(payload);
          graph.render?.();
          graph.getEdges?.().forEach((edge) => graph.refreshItem?.(edge));
          graph.refreshPositions?.();
          graph.paint?.();
          if (viewport) safeCall(applyGraphViewport, viewport);
          safeCall(ensureGraphDataVisible, graph, nodes, edges, reason || "force-redraw");
          safeCall(applyStyleToGraph, state?.graphStyle, { syncView: false });
          safeCall(applyAnalysisEncodings);
          if (state?.edgeDetail) safeCall(applyEdgeDetailLabels, true);
          safeCall(syncEdgeDetailButton);
          safeCall(updateMinimap);
          if (restoreMask) {
            requestAnimationFrame(() => {
              try {
                graph.paint?.();
              } catch (e) {}
              requestAnimationFrame(() => restoreMask());
            });
          }
          if (logNormal) {
            safeCall(logGhostProbe, "force-redraw-done", {
              reason: reason || "force-redraw",
              restoreViewport: !!restoreViewport,
              hideDuringRedraw: !!hideDuringRedraw,
              before: countsBefore,
              after: safeCounts(safeCall(collectGhostProbeCounts, graph)),
            });
          }
          return true;
        } catch (e) {
          if (restoreMask) restoreMask();
          safeCall(
            logGhostProbe,
            "force-redraw-failed",
            {
              reason: reason || "force-redraw",
              restoreViewport: !!restoreViewport,
              hideDuringRedraw: !!hideDuringRedraw,
              before: countsBefore,
              message: String(e?.message || e),
            },
            { level: "WARN" }
          );
          return false;
        }
      });
    }

    function recreateGraphInstanceFromState(reason = "", { restoreViewport = true, hideDuringRebuild = false } = {}) {
      const prevGraph = state?.graph?.instance || null;
      const data = state?.graph?.data || { nodes: [], edges: [] };
      const nodes = Array.isArray(data.nodes) ? data.nodes : [];
      const edges = Array.isArray(data.edges) ? data.edges : [];
      const viewport = restoreViewport && prevGraph ? safeCall(captureGraphViewport, prevGraph) : null;
      const restoreMask = prepareGraphRedrawMask(prevGraph, `${reason || "recreate"}-rebuild`, !!hideDuringRebuild);
      const logNormal = !safeCall(isDeleteReason, reason) || safeCall(isDeleteProbeVerbose);
      if (logNormal) {
        safeCall(logGhostProbe, "graph-recreate-start", {
          reason: reason || "mutation",
          restoreViewport: !!restoreViewport,
          hideDuringRebuild: !!hideDuringRebuild,
          before: safeCounts(safeCall(collectGhostProbeCounts, prevGraph)),
        });
      }
      try {
        safeCall(beforeGraphRecreate, prevGraph, reason);
        safeCall(destroyMinimap);
        const container = prevGraph?.get?.("container") || document.querySelector("#graphContainer");
        if (prevGraph) {
          try {
            prevGraph.clear?.();
          } catch (e) {}
          try {
            prevGraph.destroy?.();
          } catch (e) {}
        }
        if (state?.graph) {
          state.graph.instance = null;
          state.graph.inited = false;
        }
        if (container && typeof container.innerHTML === "string") {
          container.innerHTML = "";
        }
        const nextGraph = safeCall(ensureGraph);
        if (!nextGraph) {
          if (restoreMask) restoreMask();
          safeCall(
            logGhostProbe,
            "graph-recreate-failed",
            {
              reason: reason || "mutation",
              restoreViewport: !!restoreViewport,
              hideDuringRebuild: !!hideDuringRebuild,
              message: "ensureGraph failed",
            },
            { level: "WARN" }
          );
          return false;
        }
        const payload =
          typeof cloneGraphData === "function" ? cloneGraphData({ nodes, edges }) : { nodes: [...nodes], edges: [...edges] };
        nextGraph.data(payload);
        nextGraph.render?.();
        nextGraph.getEdges?.().forEach((edge) => nextGraph.refreshItem?.(edge));
        nextGraph.refreshPositions?.();
        nextGraph.paint?.();
        if (viewport) safeCall(applyGraphViewport, viewport, nextGraph);
        safeCall(ensureGraphDataVisible, nextGraph, nodes, edges, `${reason || "recreate"}-rehydrate`);
        safeCall(applyStyleToGraph, state?.graphStyle, { syncView: false });
        safeCall(applyAnalysisEncodings);
        if (state?.edgeDetail) safeCall(applyEdgeDetailLabels, true);
        safeCall(syncEdgeDetailButton);
        safeCall(updateMinimap);
        if (restoreMask) {
          requestAnimationFrame(() => {
            try {
              nextGraph.paint?.();
            } catch (e) {}
            requestAnimationFrame(() => restoreMask());
          });
        }
        if (logNormal) {
          safeCall(logGhostProbe, "graph-recreate-done", {
            reason: reason || "mutation",
            restoreViewport: !!restoreViewport,
            hideDuringRebuild: !!hideDuringRebuild,
            after: safeCounts(safeCall(collectGhostProbeCounts, nextGraph)),
          });
        }
        return true;
      } catch (e) {
        if (restoreMask) restoreMask();
        safeCall(
          logGhostProbe,
          "graph-recreate-failed",
          {
            reason: reason || "mutation",
            restoreViewport: !!restoreViewport,
            hideDuringRebuild: !!hideDuringRebuild,
            message: String(e?.message || e),
          },
          { level: "WARN" }
        );
        return false;
      }
    }

    function runGraphConsistencyCheck(reason = "") {
      const graph = state?.graph?.instance || null;
      if (!graph) return false;
      if (state?.graphLoading || state?.edgeBatching) {
        safeCall(
          logGhostProbe,
          "consistency-skip",
          { reason: reason || "afterrender", skip: state?.graphLoading ? "graphLoading" : "edgeBatching" },
          {
            dedupKey: `consistency-skip|${state?.graphMutationSeq || 0}|${reason || ""}|loading-batching`,
            dedupMs: 220,
            onlyInDeleteWindow: true,
          }
        );
        return false;
      }
      if (state?.introAnimating || Date.now() < Number(state?.introSuppressUntil || 0)) {
        safeCall(
          logGhostProbe,
          "consistency-skip",
          { reason: reason || "afterrender", skip: "introAnimating" },
          {
            dedupKey: `consistency-skip|${state?.graphMutationSeq || 0}|${reason || ""}|intro`,
            dedupMs: 220,
            onlyInDeleteWindow: true,
          }
        );
        return false;
      }

      const data = state?.graph?.data || { nodes: [], edges: [] };
      const dataNodes = Array.isArray(data.nodes) ? data.nodes : [];
      const dataEdges = Array.isArray(data.edges) ? data.edges : [];
      const expectedNodeIds = new Set(dataNodes.map((n) => String(n?.id || "").trim()).filter(Boolean));
      const expectedEdgeKeys = new Set(dataEdges.map((e) => safeCall(graphEdgeIdentityKey, e)).filter(Boolean));

      const runtimeNodes = graph.getNodes?.() || [];
      const runtimeEdges = graph.getEdges?.() || [];
      let mismatch = false;
      const strayNodeIds = [];
      const strayEdgeIds = [];

      if (runtimeNodes.length !== expectedNodeIds.size) mismatch = true;
      if (runtimeEdges.length !== expectedEdgeKeys.size) mismatch = true;

      runtimeNodes.forEach((item) => {
        const id = String(item?.getModel?.()?.id || "").trim();
        if (!id || !expectedNodeIds.has(id)) {
          mismatch = true;
          if (strayNodeIds.length < 8) strayNodeIds.push(id || "<empty>");
        }
      });
      runtimeEdges.forEach((item) => {
        const model = item?.getModel?.() || {};
        const key = safeCall(graphEdgeIdentityKey, model) || "";
        if (!key || !expectedEdgeKeys.has(key)) {
          mismatch = true;
          if (strayEdgeIds.length < 8) {
            strayEdgeIds.push(String(model?.id || `${model?.source || ""}->${model?.target || ""}`).trim() || "<empty>");
          }
        }
      });
      if (!mismatch) {
        if (safeCall(shouldLogConsistencyProbeNormal, reason)) {
          safeCall(
            logGhostProbe,
            "consistency-pass",
            {
              reason: reason || "afterrender",
              expect: { nodes: expectedNodeIds.size, edges: expectedEdgeKeys.size },
              runtime: { nodes: runtimeNodes.length, edges: runtimeEdges.length },
            },
            {
              dedupKey: `consistency-pass|${state?.graphMutationSeq || 0}|${reason || ""}`,
              dedupMs: 180,
              onlyInDeleteWindow: true,
            }
          );
        }
        return false;
      }

      const now = Date.now();
      if (now < Number(state?.graphConsistencyMuteUntil || 0)) return false;
      if (state) state.graphConsistencyMuteUntil = now + 220;
      safeCall(
        logGhostProbe,
        "consistency-mismatch",
        {
          reason: reason || "afterrender",
          expect: { nodes: expectedNodeIds.size, edges: expectedEdgeKeys.size },
          runtime: { nodes: runtimeNodes.length, edges: runtimeEdges.length },
          strayNodeIds,
          strayEdgeIds,
        },
        {
          level: "WARN",
          dedupKey: `consistency-mismatch|${state?.graphMutationSeq || 0}|${reason || ""}`,
          dedupMs: 180,
        }
      );
      safeCall(log, "WARN", "graph consistency mismatch", {
        reason: reason || "afterrender",
        expect: { nodes: expectedNodeIds.size, edges: expectedEdgeKeys.size },
        runtime: { nodes: runtimeNodes.length, edges: runtimeEdges.length },
        strayNodeIds,
        strayEdgeIds,
      });
      safeCall(cancelGraphTransientTasks, "consistency-mismatch");
      const redrawOk = forceRedrawGraphFromState("consistency-mismatch", { restoreViewport: true });
      if (!redrawOk || !safeCall(isGraphRuntimeStable, state?.graph?.instance)) {
        recreateGraphInstanceFromState("consistency-mismatch", {
          restoreViewport: true,
          hideDuringRebuild: true,
        });
      }
      return true;
    }

    function scheduleGraphConsistencyCheck(reason = "") {
      if (state?.graphConsistencyRaf) return;
      state.graphConsistencyRaf = requestAnimationFrame(() => {
        if (state) state.graphConsistencyRaf = 0;
        runGraphConsistencyCheck(reason);
      });
    }

    function scheduleDeleteVisualGuard(reason = "", delays = []) {
      const mutationSeq = Number(state?.graphMutationSeq || 0);
      const seq = Number(state?.deleteVisualGuardSeq || 0) + 1;
      if (state) state.deleteVisualGuardSeq = seq;
      if (Array.isArray(state?.deleteVisualGuardTimers) && state.deleteVisualGuardTimers.length) {
        state.deleteVisualGuardTimers.forEach((timer) => {
          try {
            clearTimeout(timer);
          } catch (e) {}
        });
      }
      if (state) state.deleteVisualGuardTimers = [];
      const list = Array.isArray(delays) && delays.length ? delays : [60, 180, 420, 900, 1500];
      safeCall(logGhostProbe, "visual-guard-schedule", {
        reason: reason || "delete-selected",
        seq,
        mutationSeq,
        delays: list.slice(),
      });
      list.forEach((msRaw) => {
        const ms = Math.max(0, Number(msRaw) || 0);
        const timer = setTimeout(() => {
          if (seq !== Number(state?.deleteVisualGuardSeq || 0)) {
            safeCall(
              logGhostProbe,
              "visual-guard-skip",
              { reason: reason || "delete-selected", seq, currentSeq: state?.deleteVisualGuardSeq || 0, mutationSeq, ms, skip: "guard-seq" },
              { dedupKey: `visual-guard-skip|${seq}|${ms}|guard-seq`, dedupMs: 120, onlyInDeleteWindow: true }
            );
            return;
          }
          if (Number(state?.graphMutationSeq || 0) !== mutationSeq) {
            safeCall(
              logGhostProbe,
              "visual-guard-skip",
              { reason: reason || "delete-selected", seq, mutationSeq, currentMutationSeq: state?.graphMutationSeq || 0, ms, skip: "mutation-seq" },
              { dedupKey: `visual-guard-skip|${seq}|${ms}|mutation-seq`, dedupMs: 120, onlyInDeleteWindow: true }
            );
            return;
          }
          if (state?.graphLoading || state?.edgeBatching || state?.introAnimating) {
            safeCall(
              logGhostProbe,
              "visual-guard-skip",
              {
                reason: reason || "delete-selected",
                seq,
                mutationSeq,
                ms,
                skip: state?.graphLoading ? "graphLoading" : state?.edgeBatching ? "edgeBatching" : "introAnimating",
              },
              { dedupKey: `visual-guard-skip|${seq}|${ms}|busy`, dedupMs: 120, onlyInDeleteWindow: true }
            );
            return;
          }
          const graph = safeCall(ensureGraph);
          if (!graph) return;
          const renderer = String(graph.getRenderer?.() || graph.get?.("renderer") || "").toLowerCase();
          if (renderer !== "webgl") {
            safeCall(
              logGhostProbe,
              "visual-guard-skip",
              { reason: reason || "delete-selected", seq, mutationSeq, ms, skip: "non-webgl", renderer },
              { dedupKey: `visual-guard-skip|${seq}|${ms}|non-webgl`, dedupMs: 120, onlyInDeleteWindow: true }
            );
            return;
          }
          let painted = false;
          try {
            graph.refreshPositions?.();
            graph.paint?.();
            painted = true;
          } catch (e) {}
          requestAnimationFrame(() => {
            if (seq !== Number(state?.deleteVisualGuardSeq || 0)) return;
            if (Number(state?.graphMutationSeq || 0) !== mutationSeq) return;
            try {
              graph.paint?.();
            } catch (e) {}
          });
          safeCall(
            logGhostProbe,
            "visual-guard-run",
            { reason: reason || "delete-selected", seq, mutationSeq, ms, renderer, painted },
            {
              dedupKey: `visual-guard-run|${seq}|${ms}`,
              dedupMs: 100,
              onlyInDeleteWindow: true,
            }
          );
        }, ms);
        state?.deleteVisualGuardTimers?.push(timer);
      });
    }

    function runGraphVisualSweep(reason = "", frames = 2) {
      const graph = safeCall(ensureGraph);
      if (!graph) return false;
      const renderer = String(graph.getRenderer?.() || graph.get?.("renderer") || "").toLowerCase();
      if (renderer !== "webgl") return false;
      const extraFrames = Math.max(0, Number(frames) || 0);
      const paintNow = () => {
        try {
          graph.paint?.();
          return true;
        } catch (e) {
          return false;
        }
      };
      const ok = paintNow();
      const sweep = (left) => {
        if (left <= 0) return;
        requestAnimationFrame(() => {
          paintNow();
          sweep(left - 1);
        });
      };
      sweep(extraFrames);
      if (safeCall(shouldLogConsistencyProbeNormal, reason)) {
        safeCall(
          logGhostProbe,
          "visual-sweep-run",
          {
            reason: reason || "mutation",
            renderer,
            frames: extraFrames,
            painted: ok,
          },
          {
            dedupKey: `visual-sweep-run|${state?.graphMutationSeq || 0}|${reason || ""}`,
            dedupMs: 120,
            onlyInDeleteWindow: true,
          }
        );
      }
      return ok;
    }

    function scheduleGraphMutationReconcile(reason = "", delays = [80, 260, 720, 1100, 1600]) {
      if (state) state.graphMutationSeq += 1;
      const seq = Number(state?.graphMutationSeq || 0);
      const list = Array.isArray(delays) ? delays : [180, 900, 1500];
      safeCall(logGhostProbe, "mutation-reconcile-schedule", { reason: reason || "mutation", seq, delays: list.slice() });
      list.forEach((msRaw) => {
        const ms = Math.max(0, Number(msRaw) || 0);
        setTimeout(() => {
          if (seq !== Number(state?.graphMutationSeq || 0)) {
            safeCall(
              logGhostProbe,
              "mutation-reconcile-skip",
              {
                reason: reason || "mutation",
                seq,
                currentSeq: state?.graphMutationSeq || 0,
                ms,
                skip: "seq-mismatch",
              },
              {
                dedupKey: `mutation-reconcile-skip|${seq}|${ms}|seq`,
                dedupMs: 180,
                onlyInDeleteWindow: true,
              }
            );
            return;
          }
          if (state?.graphLoading || state?.edgeBatching) {
            safeCall(
              logGhostProbe,
              "mutation-reconcile-skip",
              {
                reason: reason || "mutation",
                seq,
                currentSeq: state?.graphMutationSeq || 0,
                ms,
                skip: state?.graphLoading ? "graphLoading" : "edgeBatching",
              },
              {
                dedupKey: `mutation-reconcile-skip|${seq}|${ms}|load-batch`,
                dedupMs: 180,
                onlyInDeleteWindow: true,
              }
            );
            return;
          }
          const graph = safeCall(ensureGraph);
          const mismatchHandled = runGraphConsistencyCheck(`${reason || "mutation"}@${ms}ms`);
          if (mismatchHandled) {
            safeCall(
              logGhostProbe,
              "mutation-reconcile-skip",
              {
                reason: reason || "mutation",
                seq,
                currentSeq: state?.graphMutationSeq || 0,
                ms,
                skip: "consistency-mismatch-handled",
              },
              {
                dedupKey: `mutation-reconcile-skip|${seq}|${ms}|consistency-mismatch-handled`,
                dedupMs: 180,
              }
            );
            return;
          }
          if (graph && safeCall(isGraphRuntimeStable, graph)) {
            safeCall(
              logGhostProbe,
              "mutation-reconcile-skip",
              {
                reason: reason || "mutation",
                seq,
                currentSeq: state?.graphMutationSeq || 0,
                ms,
                skip: "stable-runtime",
              },
              {
                dedupKey: `mutation-reconcile-skip|${seq}|${ms}|stable-runtime`,
                dedupMs: 180,
              }
            );
            return;
          }
          const redrawOk = forceRedrawGraphFromState(`${reason || "mutation"}@${ms}ms`, { restoreViewport: true });
          safeCall(logGhostProbe, "mutation-reconcile-run", { reason: reason || "mutation", seq, ms, redrawOk });
        }, ms);
      });
    }

    return {
      forceRedrawGraphFromState,
      recreateGraphInstanceFromState,
      runGraphConsistencyCheck,
      scheduleGraphConsistencyCheck,
      scheduleDeleteVisualGuard,
      runGraphVisualSweep,
      scheduleGraphMutationReconcile,
      shouldRunDeleteVisualGuard: () => !!safeCall(shouldRunDeleteVisualGuard),
      shouldAlwaysRecreateAfterDelete: () => !!safeCall(shouldAlwaysRecreateAfterDelete),
    };
  }

  window.__ANALYTIX_FLOW_CANVAS_REDRAW_CONTROLLER__ = {
    createCanvasRedrawController,
  };
})();
