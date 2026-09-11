/* Layout meta model.
 * Responsibilities: clear transient layout metadata inside the synchronous layout kernel.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return;

  const layoutWorkerContract = root.__ANALYTIX_LAYOUT_WORKER_CONTRACT__;
  if (
    !layoutWorkerContract ||
    !Array.isArray(layoutWorkerContract.LAYOUT_META_KEYS)
  ) {
    throw new Error("layout-meta-contract-missing");
  }

  const LAYOUT_META_KEYS = [...layoutWorkerContract.LAYOUT_META_KEYS];

  function clearLayoutMeta(nodes, metaKeys = LAYOUT_META_KEYS) {
    (Array.isArray(nodes) ? nodes : []).forEach((node) => {
      if (!node || typeof node !== "object") return;
      (Array.isArray(metaKeys) ? metaKeys : []).forEach((key) => {
        if (key in node) delete node[key];
      });
    });
  }

  root.__ANALYTIX_FLOW_LAYOUT_META_MODEL__ = {
    LAYOUT_META_KEYS,
    clearLayoutMeta,
  };
})();
