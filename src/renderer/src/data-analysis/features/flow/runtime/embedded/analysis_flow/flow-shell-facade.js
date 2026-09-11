(() => {
  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function createFlowShellFacade({
    runtimeStore = null,
    listeners = null,
    buildShellState = null,
    getPerfSnapshot = null,
    finalizeShellCommandRegistry = null,
    commandDefinitions = null,
  } = {}) {
    function getState() {
      if (runtimeStore && typeof runtimeStore.getShellState === "function") {
        const snapshot = runtimeStore.getShellState();
        if (snapshot && typeof snapshot === "object" && Object.keys(snapshot).length) {
          return snapshot;
        }
      }
      return typeof buildShellState === "function" ? buildShellState() : {};
    }

    function subscribe(listener) {
      if (typeof listener !== "function") {
        return () => {};
      }
      if (listeners && typeof listeners.add === "function") {
        listeners.add(listener);
      }
      if (runtimeStore && typeof runtimeStore.subscribeShellState === "function") {
        const snapshot = typeof runtimeStore.getShellState === "function" ? runtimeStore.getShellState() : null;
        if (!snapshot || typeof snapshot !== "object" || !Object.keys(snapshot).length) {
          try {
            listener(getState(), { reason: "subscribe" });
          } catch {}
        }
        const unsubscribeRuntimeStore = runtimeStore.subscribeShellState(listener);
        return () => {
          if (listeners && typeof listeners.delete === "function") {
            listeners.delete(listener);
          }
          if (typeof unsubscribeRuntimeStore === "function") {
            unsubscribeRuntimeStore();
          }
        };
      }
      try {
        listener(getState(), { reason: "subscribe" });
      } catch {}
      return () => {
        if (listeners && typeof listeners.delete === "function") {
          listeners.delete(listener);
        }
      };
    }

    return {
      getState,
      getPerfSnapshot() {
        return typeof getPerfSnapshot === "function" ? getPerfSnapshot() : {};
      },
      subscribe,
      commands:
        typeof finalizeShellCommandRegistry === "function"
          ? finalizeShellCommandRegistry(commandDefinitions || {})
          : commandDefinitions || {},
      debug: {
        source: "flow-shell-facade",
        runtimeStore: !!runtimeStore,
        listenerCount: listeners && typeof listeners.size === "number" ? listeners.size : 0,
      },
      toJSON() {
        return {
          source: "flow-shell-facade",
          hasRuntimeStore: !!runtimeStore,
        };
      },
      label: text("viz-shell"),
    };
  }

  window.__ANALYTIX_FLOW_SHELL_FACADE__ = {
    createFlowShellFacade,
  };
})();
