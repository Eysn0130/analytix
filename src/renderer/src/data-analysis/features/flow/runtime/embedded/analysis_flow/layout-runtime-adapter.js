/* Layout runtime adapter.
 * Responsibilities: resolve layout core exports and create layout workers only.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  const DEFAULT_LAYOUT_ALGO_VERSION = "layout-core-unknown";
  const LAYOUT_WORKER_SCRIPT = "layout_worker.js";

  function getLayoutEngine() {
    const engine = root.AnalytixLayoutEngine || null;
    return engine && typeof engine === "object" ? engine : null;
  }

  function getLayoutAlgoVersion(engine = getLayoutEngine()) {
    return String(engine?.LAYOUT_ALGO_VERSION || DEFAULT_LAYOUT_ALGO_VERSION);
  }

  function createLayoutWorkerScript(version = getLayoutAlgoVersion()) {
    const normalized = String(version || DEFAULT_LAYOUT_ALGO_VERSION);
    return `${LAYOUT_WORKER_SCRIPT}?v=${encodeURIComponent(normalized)}`;
  }

  function getLayoutMetaKeys(engine = getLayoutEngine()) {
    return Array.isArray(engine?.LAYOUT_META_KEYS) ? engine.LAYOUT_META_KEYS.slice() : [];
  }

  function callLayoutCore(method, args = [], options = {}) {
    const name = String(method || "");
    const engine = getLayoutEngine();
    const fn = engine?.[name];
    if (typeof fn === "function") return fn(...(Array.isArray(args) ? args : []));
    if (typeof options?.onMissing === "function") options.onMissing(name);
    return undefined;
  }

  function buildLayoutWorkerRequestPayload(payload = {}, options = {}) {
    const request = callLayoutCore("buildLayoutWorkerRequest", [payload], options);
    if (!request || typeof request !== "object") {
      throw new Error("layout-worker-request-contract-missing");
    }
    return request;
  }

  function clonePrewarmLayoutInput(nodes, edges, options = {}) {
    const input = callLayoutCore("clonePrewarmLayoutInput", [nodes, edges], options);
    if (!input || !Array.isArray(input.clonedNodes) || !Array.isArray(input.clonedEdges)) {
      throw new Error("layout-prewarm-input-contract-missing");
    }
    return input;
  }

  function createLayoutWorker({ script = createLayoutWorkerScript(), WorkerCtor = root.Worker } = {}) {
    if (typeof WorkerCtor !== "function") return null;
    return new WorkerCtor(String(script || createLayoutWorkerScript()));
  }

  root.__ANALYTIX_FLOW_LAYOUT_RUNTIME_ADAPTER__ = {
    DEFAULT_LAYOUT_ALGO_VERSION,
    LAYOUT_WORKER_SCRIPT,
    getLayoutEngine,
    getLayoutAlgoVersion,
    createLayoutWorkerScript,
    getLayoutMetaKeys,
    callLayoutCore,
    buildLayoutWorkerRequestPayload,
    clonePrewarmLayoutInput,
    createLayoutWorker,
  };
})();
