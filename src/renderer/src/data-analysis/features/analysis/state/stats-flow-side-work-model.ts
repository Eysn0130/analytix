import type { StatsWorkspaceTab } from "../api/stats-api";

export interface StatsFlowSideWorkState {
  activeCaseId: string;
  activeWorkspaceTab: StatsWorkspaceTab;
  loadingRows: boolean;
  loadingTxn?: boolean;
  statsPageStateReady?: boolean;
  txnModalOpen?: boolean;
}

export function shouldRunStatsFlowSideWork(state: StatsFlowSideWorkState): boolean {
  return (
    Boolean(String(state.activeCaseId || "").trim()) &&
    state.activeWorkspaceTab === "stats" &&
    !state.loadingRows &&
    !state.loadingTxn &&
    !state.txnModalOpen &&
    state.statsPageStateReady !== false
  );
}
