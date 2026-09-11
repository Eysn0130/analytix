(() => {
  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function createFlowShellSyncStore({ runtimeStore = null, listeners = null, buildShellState = null, targetWindow = window } = {}) {
    let syncToken = 0;
    let pendingReason = "";

    function cancel() {
      if (!syncToken) return;
      if (typeof targetWindow.cancelAnimationFrame === "function") {
        targetWindow.cancelAnimationFrame(syncToken);
      } else {
        clearTimeout(syncToken);
      }
      syncToken = 0;
    }

    function flush(reason = "") {
      cancel();
      const snapshot = typeof buildShellState === "function" ? buildShellState() : {};
      if (runtimeStore && typeof runtimeStore.publishShellState === "function") {
        try {
          runtimeStore.publishShellState(snapshot, { reason: text(reason) });
        } catch {
          // noop
        }
      }
      pendingReason = "";
      if (listeners && typeof listeners.forEach === "function") {
        listeners.forEach((listener) => {
          try {
            listener(snapshot, { reason: text(reason) });
          } catch {
            // noop
          }
        });
      }
      return snapshot;
    }

    function schedule(reason = "") {
      pendingReason = text(reason || pendingReason || "");
      if (syncToken) return;
      const run = () => {
        syncToken = 0;
        flush(pendingReason);
      };
      if (typeof targetWindow.requestAnimationFrame === "function") {
        syncToken = targetWindow.requestAnimationFrame(run);
      } else {
        syncToken = targetWindow.setTimeout(run, 0);
      }
    }

    return {
      cancel,
      flush,
      schedule,
      getSnapshot() {
        return {
          pendingReason,
          hasPending: !!syncToken,
        };
      },
    };
  }

  window.__ANALYTIX_FLOW_SHELL_SYNC_STORE__ = {
    createFlowShellSyncStore,
  };
})();
