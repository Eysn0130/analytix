(() => {
  function initializeTxnModalState(modal, { title, model, lane, requestKey, expectedTotal }) {
    if (!modal) return 0;
    modal.reqId = (modal.reqId || 0) + 1;
    modal.open = true;
    modal.title = title || "交易明细";
    modal.model = model || null;
    modal.lane = lane || "";
    modal.requestKey = requestKey || "";
    modal.rows = [];
    modal.loading = true;
    modal.error = "";
    modal.sortCol = "txn_time";
    modal.sortDir = "asc";
    modal.expectedTotal = Math.max(0, Number(expectedTotal) || 0);
    return modal.reqId;
  }

  function resetTxnModalState(modal) {
    if (!modal) return 0;
    modal.reqId = (modal.reqId || 0) + 1;
    modal.open = false;
    modal.title = "交易明细";
    modal.model = null;
    modal.lane = "";
    modal.requestKey = "";
    modal.rows = [];
    modal.loading = false;
    modal.error = "";
    modal.expectedTotal = 0;
    return modal.reqId;
  }

  function toggleTxnModalSortState(modal, colKey, isTxnSortableCol) {
    const key = String(colKey || "");
    if (!modal || typeof isTxnSortableCol !== "function" || !isTxnSortableCol(key)) return false;
    const same = modal.sortCol === key;
    modal.sortCol = key;
    modal.sortDir = same ? (modal.sortDir === "asc" ? "desc" : "asc") : key === "amount" ? "desc" : "asc";
    return true;
  }

  window.__ANALYTIX_FLOW_TXN_MODAL_CONTROLLER__ = {
    initializeTxnModalState,
    resetTxnModalState,
    toggleTxnModalSortState,
  };
})();
