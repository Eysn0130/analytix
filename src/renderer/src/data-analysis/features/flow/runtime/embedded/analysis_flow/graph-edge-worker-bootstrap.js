/* global document, window, Worker */
(() => {
  const currentScript = document.currentScript;
  const baseUrl =
    currentScript && currentScript.src ? new URL("./", currentScript.src).toString() : new URL("./", window.location.href).toString();
  const workerUrl = new URL("graph-edge-worker.runtime.js", baseUrl).toString();

  function createEdgeWorker(onMessage, onError) {
    const worker = new Worker(workerUrl);
    worker.onmessage = typeof onMessage === "function" ? onMessage : null;
    worker.onerror = typeof onError === "function" ? onError : null;
    return worker;
  }

  window.__ANALYTIX_GRAPH_EDGE_WORKER_BOOTSTRAP__ = {
    createEdgeWorker,
    workerUrl,
  };
})();
