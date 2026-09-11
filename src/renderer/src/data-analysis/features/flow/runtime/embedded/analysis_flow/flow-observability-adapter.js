(() => {
  const PERF_BASELINES = {
    layoutDurationMs: 1200,
    patchPublishMs: 80,
    patchApplyMs: 120,
    textAtlasPeakTextureBytes: 64 * 1024 * 1024,
  };
  const PERF_WINDOW_LIMIT = 24;
  const ALERT_HISTORY_LIMIT = 20;

  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function asNumber(value, fallback = 0) {
    const next = Number(value);
    return Number.isFinite(next) ? next : fallback;
  }

  function createFlowPerfMetrics() {
    return {
      layoutWorker: {
        startedCount: 0,
        completedCount: 0,
        cancelledCount: 0,
        fallbackCount: 0,
        failedCount: 0,
        lastSeq: 0,
        lastStage: "",
        lastReason: "",
        lastPreset: "",
        lastNodeCount: 0,
        lastEdgeCount: 0,
        lastDurationMs: 0,
        lastStartedAt: 0,
        lastCompletedAt: 0,
        lastTraceId: "",
      },
      textAtlas: {
        sampledCount: 0,
        lastSampleAt: 0,
        lastTraceId: "",
        current: {
          setCount: 0,
          atlasCount: 0,
          glyphCount: 0,
          approxTextureBytes: 0,
          batchCount: 0,
        },
        peaks: {
          setCount: 0,
          atlasCount: 0,
          glyphCount: 0,
          approxTextureBytes: 0,
          batchCount: 0,
        },
      },
      observability: {
        windows: {
          layoutDuration: createSampleWindow(),
          patchPublish: createSampleWindow(),
          patchApply: createSampleWindow(),
          buildDuration: createSampleWindow(),
          textAtlasPeakTextureBytes: createSampleWindow(),
        },
        alertHistory: [],
        lastAlertToken: "",
      },
    };
  }

  function createSampleWindow(limit = PERF_WINDOW_LIMIT) {
    return {
      limit: Math.max(1, asNumber(limit, PERF_WINDOW_LIMIT)),
      lastToken: "",
      samples: [],
    };
  }

  function pushSample(windowState, { value = 0, at = 0, traceId = "", phase = "", reason = "" } = {}) {
    if (!windowState || typeof windowState !== "object") {
      return false;
    }
    const normalizedValue = Math.max(0, asNumber(value, NaN));
    if (!Number.isFinite(normalizedValue)) {
      return false;
    }
    const normalizedAt = Math.max(0, asNumber(at, Date.now()));
    const normalizedTraceId = text(traceId);
    const normalizedPhase = text(phase);
    const normalizedReason = text(reason);
    const token = [normalizedAt, normalizedTraceId, normalizedValue, normalizedPhase, normalizedReason].join("|");
    if (!token || token === windowState.lastToken) {
      return false;
    }
    windowState.lastToken = token;
    const samples = Array.isArray(windowState.samples) ? windowState.samples : [];
    samples.push({
      value: normalizedValue,
      at: normalizedAt,
      traceId: normalizedTraceId,
      phase: normalizedPhase,
      reason: normalizedReason,
    });
    while (samples.length > Math.max(1, asNumber(windowState.limit, PERF_WINDOW_LIMIT))) {
      samples.shift();
    }
    windowState.samples = samples;
    return true;
  }

  function summarizeWindow(windowState) {
    const samples = Array.isArray(windowState?.samples) ? windowState.samples : [];
    if (!samples.length) {
      return {
        count: 0,
        avg: 0,
        min: 0,
        max: 0,
        latest: 0,
        firstAt: 0,
        lastAt: 0,
        windowMs: 0,
        traceId: "",
      };
    }
    const values = samples.map((item) => Math.max(0, asNumber(item?.value, 0)));
    const firstAt = Math.max(0, asNumber(samples[0]?.at, 0));
    const lastSample = samples[samples.length - 1] || {};
    const lastAt = Math.max(firstAt, asNumber(lastSample.at, firstAt));
    const sum = values.reduce((acc, value) => acc + value, 0);
    return {
      count: samples.length,
      avg: Math.round((sum / Math.max(1, samples.length)) * 100) / 100,
      min: Math.min(...values),
      max: Math.max(...values),
      latest: values[values.length - 1] || 0,
      firstAt,
      lastAt,
      windowMs: Math.max(0, lastAt - firstAt),
      traceId: text(lastSample.traceId),
    };
  }

  function appendAlertHistory(metrics, alerts = [], timestamp = Date.now()) {
    const observability =
      metrics && metrics.observability && typeof metrics.observability === "object" ? metrics.observability : null;
    if (!observability) {
      return [];
    }
    const history = Array.isArray(observability.alertHistory) ? observability.alertHistory : [];
    const nextAlerts = Array.isArray(alerts) ? alerts : [];
    const token = nextAlerts
      .map((alert) => {
        const row = alert && typeof alert === "object" ? alert : {};
        return [text(row.id), asNumber(row.value), text(row.traceId), text(row.message)].join(":");
      })
      .join("|");
    if (token && token !== text(observability.lastAlertToken)) {
      history.push({
        at: Math.max(0, asNumber(timestamp, Date.now())),
        alerts: nextAlerts.map((item) => ({ ...(item || {}) })),
      });
      while (history.length > ALERT_HISTORY_LIMIT) {
        history.shift();
      }
      observability.lastAlertToken = token;
    }
    observability.alertHistory = history;
    return history.slice();
  }

  function ingestMetricWindow(windowState, bucket = {}, { phase = "", reason = "" } = {}) {
    const source = bucket && typeof bucket === "object" ? bucket : {};
    pushSample(windowState, {
      value: source.lastDurationMs,
      at: source.lastAt,
      traceId: source.lastTraceId,
      phase,
      reason: reason || source.lastReason || source.lastStatus || source.lastKind || source.lastScope,
    });
  }

  function recordLayoutWorker(metrics, event, details = {}) {
    const target = metrics && metrics.layoutWorker ? metrics.layoutWorker : null;
    if (!target) {
      return;
    }
    const type = text(event).toLowerCase();
    target.lastSeq = Math.max(0, asNumber(details.seq, target.lastSeq));
    target.lastStage = text(details.stage || target.lastStage);
    target.lastReason = text(details.reason || target.lastReason);
    target.lastPreset = text(details.preset || target.lastPreset);
    target.lastNodeCount = Math.max(0, asNumber(details.nodes, target.lastNodeCount));
    target.lastEdgeCount = Math.max(0, asNumber(details.edges, target.lastEdgeCount));
    target.lastTraceId = text(details.traceId || details.trace_id || target.lastTraceId);
    if (type === "start") {
      target.startedCount += 1;
      target.lastStartedAt = Date.now();
    } else if (type === "complete") {
      target.completedCount += 1;
      target.lastDurationMs = Math.max(0, asNumber(details.durationMs, target.lastDurationMs));
      target.lastCompletedAt = Date.now();
      pushSample(metrics?.observability?.windows?.layoutDuration, {
        value: target.lastDurationMs,
        at: target.lastCompletedAt,
        traceId: target.lastTraceId,
        phase: "complete",
        reason: target.lastReason || target.lastStage,
      });
    } else if (type === "cancel") {
      target.cancelledCount += 1;
    } else if (type === "fallback") {
      target.fallbackCount += 1;
      target.lastDurationMs = Math.max(0, asNumber(details.durationMs, target.lastDurationMs));
      target.lastCompletedAt = Date.now();
      pushSample(metrics?.observability?.windows?.layoutDuration, {
        value: target.lastDurationMs,
        at: target.lastCompletedAt,
        traceId: target.lastTraceId,
        phase: "fallback",
        reason: target.lastReason || target.lastStage,
      });
    } else if (type === "error") {
      target.failedCount += 1;
    } else if (type === "progress") {
      target.lastStage = text(details.stage || target.lastStage);
    }
  }

  function recordTextAtlas(metrics, snapshot = null) {
    const target = metrics && metrics.textAtlas ? metrics.textAtlas : null;
    if (!target) {
      return null;
    }
    const next = snapshot && typeof snapshot === "object" ? snapshot : {};
    const current = {
      setCount: Math.max(0, asNumber(next.setCount)),
      atlasCount: Math.max(0, asNumber(next.atlasCount)),
      glyphCount: Math.max(0, asNumber(next.glyphCount)),
      approxTextureBytes: Math.max(0, asNumber(next.approxTextureBytes)),
      batchCount: Math.max(0, asNumber(next.batchCount)),
    };
    target.sampledCount += 1;
    target.lastSampleAt = Date.now();
    target.lastTraceId = text(next.traceId || next.trace_id || target.lastTraceId);
    target.current = current;
    target.peaks = {
      setCount: Math.max(target.peaks.setCount, current.setCount),
      atlasCount: Math.max(target.peaks.atlasCount, current.atlasCount),
      glyphCount: Math.max(target.peaks.glyphCount, current.glyphCount),
      approxTextureBytes: Math.max(target.peaks.approxTextureBytes, current.approxTextureBytes),
      batchCount: Math.max(target.peaks.batchCount, current.batchCount),
    };
    pushSample(metrics?.observability?.windows?.textAtlasPeakTextureBytes, {
      value: current.approxTextureBytes,
      at: target.lastSampleAt,
      traceId: target.lastTraceId,
      phase: "sample",
      reason: "text-atlas",
    });
    return current;
  }

  function buildFlowPerfSnapshot(metrics, extra = {}) {
    const source = metrics && typeof metrics === "object" ? metrics : createFlowPerfMetrics();
    const graphPatchBridge =
      extra.graphPatchBridge && typeof extra.graphPatchBridge === "object" ? { ...extra.graphPatchBridge } : {};
    const canvasInteraction =
      extra.canvasInteraction && typeof extra.canvasInteraction === "object" ? { ...extra.canvasInteraction } : {};
    const build = extra.build && typeof extra.build === "object" ? { ...extra.build } : {};
    const transport = extra.transport && typeof extra.transport === "object" ? { ...extra.transport } : {};
    const runtimeBridge = extra.runtimeBridge && typeof extra.runtimeBridge === "object" ? { ...extra.runtimeBridge } : {};
    ingestMetricWindow(source?.observability?.windows?.patchPublish, graphPatchBridge.publish, {
      phase: "publish",
    });
    ingestMetricWindow(source?.observability?.windows?.patchApply, graphPatchBridge.apply, {
      phase: "apply",
    });
    ingestMetricWindow(source?.observability?.windows?.buildDuration, build, {
      phase: "build",
    });
    const alerts = [];
    const layoutDurationMs = asNumber(source.layoutWorker?.lastDurationMs);
    const patchPublishDurationMs = asNumber(graphPatchBridge.publish?.lastDurationMs);
    const patchApplyDurationMs = asNumber(graphPatchBridge.apply?.lastDurationMs);
    const textAtlasPeakBytes = asNumber(source.textAtlas?.peaks?.approxTextureBytes);

    if (layoutDurationMs > PERF_BASELINES.layoutDurationMs) {
      alerts.push({
        id: "layout-duration",
        level: "warn",
        value: layoutDurationMs,
        baseline: PERF_BASELINES.layoutDurationMs,
        traceId: text(source.layoutWorker?.lastTraceId),
        message: `layout ${layoutDurationMs}ms > ${PERF_BASELINES.layoutDurationMs}ms`,
      });
    }
    if (patchPublishDurationMs > PERF_BASELINES.patchPublishMs) {
      alerts.push({
        id: "patch-publish",
        level: "warn",
        value: patchPublishDurationMs,
        baseline: PERF_BASELINES.patchPublishMs,
        traceId: text(graphPatchBridge.publish?.lastTraceId),
        message: `patch publish ${patchPublishDurationMs}ms > ${PERF_BASELINES.patchPublishMs}ms`,
      });
    }
    if (patchApplyDurationMs > PERF_BASELINES.patchApplyMs) {
      alerts.push({
        id: "patch-apply",
        level: "warn",
        value: patchApplyDurationMs,
        baseline: PERF_BASELINES.patchApplyMs,
        traceId: text(graphPatchBridge.apply?.lastTraceId),
        message: `patch apply ${patchApplyDurationMs}ms > ${PERF_BASELINES.patchApplyMs}ms`,
      });
    }
    if (textAtlasPeakBytes > PERF_BASELINES.textAtlasPeakTextureBytes) {
      alerts.push({
        id: "text-atlas-peak",
        level: "warn",
        value: textAtlasPeakBytes,
        baseline: PERF_BASELINES.textAtlasPeakTextureBytes,
        traceId: text(source.textAtlas?.lastTraceId),
        message: `text atlas ${textAtlasPeakBytes}B > ${PERF_BASELINES.textAtlasPeakTextureBytes}B`,
      });
    }
    const timestamp = Date.now();
    const alertHistory = appendAlertHistory(source, alerts, timestamp);

    return {
      graphPatch: extra.graphPatch && typeof extra.graphPatch === "object" ? { ...extra.graphPatch } : {},
      graphPatchBridge: {
        publish: graphPatchBridge.publish && typeof graphPatchBridge.publish === "object" ? { ...graphPatchBridge.publish } : {},
        apply: graphPatchBridge.apply && typeof graphPatchBridge.apply === "object" ? { ...graphPatchBridge.apply } : {},
      },
      build,
      transport,
      runtimeBridge,
      canvasInteraction,
      layoutWorker: source.layoutWorker ? { ...source.layoutWorker } : {},
      textAtlas: source.textAtlas
        ? {
            sampledCount: source.textAtlas.sampledCount,
            lastSampleAt: source.textAtlas.lastSampleAt,
            lastTraceId: source.textAtlas.lastTraceId,
            current: { ...(source.textAtlas.current || {}) },
            peaks: { ...(source.textAtlas.peaks || {}) },
          }
        : {},
      windows: {
        layoutDuration: summarizeWindow(source?.observability?.windows?.layoutDuration),
        patchPublish: summarizeWindow(source?.observability?.windows?.patchPublish),
        patchApply: summarizeWindow(source?.observability?.windows?.patchApply),
        buildDuration: summarizeWindow(source?.observability?.windows?.buildDuration),
        textAtlasPeakTextureBytes: summarizeWindow(source?.observability?.windows?.textAtlasPeakTextureBytes),
      },
      traces: {
        build: {
          traceId: text(build.lastTraceId),
          status: text(build.lastStatus),
          snapshotId: text(build.lastSnapshotId),
        },
        layout: {
          traceId: text(source.layoutWorker?.lastTraceId),
          stage: text(source.layoutWorker?.lastStage),
          seq: asNumber(source.layoutWorker?.lastSeq),
        },
        patch: {
          local: {
            traceId: text(extra.graphPatch?.lastTraceId),
            kind: text(extra.graphPatch?.lastPatchKind),
            scope: text(extra.graphPatch?.lastPatchScope),
          },
          publish: {
            traceId: text(graphPatchBridge.publish?.lastTraceId),
            kind: text(graphPatchBridge.publish?.lastKind),
            scope: text(graphPatchBridge.publish?.lastScope),
          },
          apply: {
            traceId: text(graphPatchBridge.apply?.lastTraceId),
            kind: text(graphPatchBridge.apply?.lastKind),
            scope: text(graphPatchBridge.apply?.lastScope),
          },
        },
      },
      baselines: { ...PERF_BASELINES },
      alerts,
      alertHistory,
      timestamp,
    };
  }

  function exportFlowPerfSnapshot(snapshot) {
    const payload = snapshot && typeof snapshot === "object" ? snapshot : {};
    return JSON.stringify(
      {
        exportedAt: Date.now(),
        snapshot: payload,
      },
      null,
      2
    );
  }

  window.__ANALYTIX_FLOW_OBSERVABILITY_ADAPTER__ = {
    PERF_BASELINES,
    createFlowPerfMetrics,
    recordLayoutWorker,
    recordTextAtlas,
    buildFlowPerfSnapshot,
    exportFlowPerfSnapshot,
  };
})();
