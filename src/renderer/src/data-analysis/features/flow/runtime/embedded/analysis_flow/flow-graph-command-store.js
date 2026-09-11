(() => {
  function createFlowGraphCommandStore({ state = null } = {}) {
    function publish(meta = {}) {
      if (state?.graphMutationStore && typeof state.graphMutationStore.publish === "function") {
        return state.graphMutationStore.publish(meta);
      }
      return false;
    }

    function schedule(meta = {}) {
      if (state?.graphMutationStore && typeof state.graphMutationStore.schedule === "function") {
        state.graphMutationStore.schedule(meta);
        return;
      }
      publish(meta);
    }

    return {
      publish,
      schedule,
    };
  }

  window.__ANALYTIX_FLOW_GRAPH_COMMAND_STORE__ = {
    createFlowGraphCommandStore,
  };
})();
