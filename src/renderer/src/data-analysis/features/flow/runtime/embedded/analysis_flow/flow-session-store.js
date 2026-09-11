(() => {
  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function createFlowSessionStore({ state = null, deps = {} } = {}) {
    const log = typeof deps.log === "function" ? deps.log : () => {};
    const setCaseBadge = typeof deps.setCaseBadge === "function" ? deps.setCaseBadge : () => {};
    const loadTreeData = typeof deps.loadTreeData === "function" ? deps.loadTreeData : async () => {};
    const syncFundsStatus = typeof deps.syncFundsStatus === "function" ? deps.syncFundsStatus : () => {};
    const clearGraph = typeof deps.clearGraph === "function" ? deps.clearGraph : () => {};
    const loadViewsFromBackend =
      typeof deps.loadViewsFromBackend === "function" ? deps.loadViewsFromBackend : async () => {};
    let caseRevision = 0;

    function isCurrentCaseRevision(caseId, revision) {
      return revision === caseRevision && text(state?.caseId) === text(caseId);
    }

    function fetchCaseName() {
      return new Promise((resolve) => {
        if (!state?.backend || typeof state.backend.getCaseName !== "function") return resolve("");
        let done = false;
        const finish = (val) => {
          if (done) return;
          done = true;
          resolve(text(val));
        };
        const timer = setTimeout(() => finish(""), 250);
        try {
          state.backend.getCaseName((name) => {
            clearTimeout(timer);
            finish(name);
          });
        } catch {
          clearTimeout(timer);
          finish("");
        }
      });
    }

    async function updateCaseBadge(expectedCaseId = text(state?.caseId), revision = caseRevision) {
      const caseId = text(expectedCaseId);
      if (!caseId) {
        if (!isCurrentCaseRevision(caseId, revision)) return false;
        setCaseBadge("", "");
        return true;
      }
      const name = await fetchCaseName();
      if (!isCurrentCaseRevision(caseId, revision)) return false;
      setCaseBadge(name, caseId);
      return true;
    }

    function syncCaseFromBackend(reason = "") {
      if (!state?.backend || typeof state.backend.getCaseId !== "function") return;
      try {
        state.backend.getCaseId((cid) => {
          const nextId = text(cid);
          if (nextId && nextId !== state.caseId) {
            log("INFO", `syncCase(${reason || "backend"}) -> ${nextId}`);
            void setCaseId(nextId);
          }
        });
      } catch {
        log("WARN", `syncCase(${reason || "backend"}) failed`);
      }
    }

    async function setCaseId(caseId) {
      const nextId = text(caseId);
      const sameCase = nextId === text(state?.caseId);
      if (sameCase && state?.viewsLoaded) {
        return;
      }
      const revision = ++caseRevision;
      if (state) {
        state.caseId = nextId;
        state.viewsLoaded = false;
      }
      if (!sameCase) {
        clearGraph();
      }
      if (!(await updateCaseBadge(nextId, revision))) return;
      await loadTreeData(state?.tab, { resetSelection: !sameCase });
      if (!isCurrentCaseRevision(nextId, revision)) return;
      syncFundsStatus();
      await loadViewsFromBackend();
      if (!isCurrentCaseRevision(nextId, revision)) return;
    }

    return {
      fetchCaseName,
      updateCaseBadge,
      syncCaseFromBackend,
      setCaseId,
    };
  }

  window.__ANALYTIX_FLOW_SESSION_STORE__ = {
    createFlowSessionStore,
  };
})();
