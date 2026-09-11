/* Layout worker bridge.
 * Responsibilities: load layout engine modules and handle worker message dispatch only.
 */
/* eslint-disable no-restricted-globals */

const FALLBACK_LAYOUT_ALGO_VERSION = "layout-core-unknown";

try {
  if (typeof self !== "undefined" && typeof self.window === "undefined") {
    self.window = self;
  }
  importScripts(
    "layout-worker-contract.js",
    "layout-meta-model.js",
    "graph_engine.js",
    "compact.js",
    "network-rust-cluster-plan.js",
    "network.js",
    "hierarchy.js",
    "flow.js"
  );
} catch (e) {
  // Keep worker alive; runtime handler below will surface error payloads.
}

function getLayoutEngine() {
  return self.AnalytixLayoutEngine || null;
}

function getLayoutWorkerContract() {
  return self.__ANALYTIX_LAYOUT_WORKER_CONTRACT__ || null;
}

self.onmessage = (ev) => {
  const engine = getLayoutEngine();
  const contract = getLayoutWorkerContract();
  const data = ev.data || {};
  if (data.type !== "layout") return;
  if (!contract || typeof contract.buildLayoutWorkerRequest !== "function" || typeof contract.buildLayoutWorkerResult !== "function") {
    self.postMessage({
      type: "layout-result",
      seq: Number(data?.seq) || 0,
      trace_id: String(data?.trace_id || data?.traceId || ""),
      nodes: [],
      algoVersion: FALLBACK_LAYOUT_ALGO_VERSION,
      coreCount: 0,
      coreIds: [],
      error: "layout-contract-missing",
    });
    return;
  }

  const request = contract.buildLayoutWorkerRequest(data);
  const nodes = Array.isArray(request.nodes) ? request.nodes : [];
  const edges = Array.isArray(request.edges) ? request.edges : [];
  const layoutHints = request.layoutHints && typeof request.layoutHints === "object" ? request.layoutHints : {};

  if (!engine || typeof engine.applyLayout !== "function") {
    self.postMessage({
      type: "layout-result",
      seq: request.seq,
      trace_id: String(request.trace_id || ""),
      nodes,
      algoVersion: FALLBACK_LAYOUT_ALGO_VERSION,
      coreCount: 0,
      coreIds: [],
      error: "layout-core-missing",
    });
    return;
  }

  self.postMessage({
    type: "layout-progress",
    seq: request.seq,
    trace_id: String(request.trace_id || ""),
    phase: "running",
    progress: 15,
    algoVersion: String(engine.LAYOUT_ALGO_VERSION || FALLBACK_LAYOUT_ALGO_VERSION),
  });

  engine.applyLayout(
    nodes,
    edges,
    request.preset,
    request.focusId,
    request.layoutDirection || {},
    layoutHints
  );

  self.postMessage(
    {
      type: "layout-result",
      phase: "complete",
      progress: 100,
      trace_id: String(request.trace_id || ""),
      ...contract.buildLayoutWorkerResult({
        seq: request.seq,
        preset: request.preset,
        layoutDirection: request.layoutDirection || {},
        nodes,
        metaKeys:
          Array.isArray(engine.LAYOUT_META_KEYS) && engine.LAYOUT_META_KEYS.length
            ? engine.LAYOUT_META_KEYS
            : contract.LAYOUT_META_KEYS,
        algoVersion: String(engine.LAYOUT_ALGO_VERSION || FALLBACK_LAYOUT_ALGO_VERSION),
        layoutReport:
          layoutHints && typeof layoutHints.__layoutReport === "object"
            ? layoutHints.__layoutReport
            : null,
      }),
    }
  );
};
