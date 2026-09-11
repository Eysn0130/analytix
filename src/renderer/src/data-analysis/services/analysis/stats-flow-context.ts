export interface PersistedStatsFlowContexts {
  caseId: string;
  updatedAt: string;
  treeTab: string;
  mode: string;
  relationPayload: Record<string, unknown> | null;
  flowPayload: Record<string, unknown> | null;
}

const STATS_FLOW_CONTEXT_STORAGE_PREFIX = "analytix:stats:flow-context";

function storageKey(caseId: string): string {
  return `${STATS_FLOW_CONTEXT_STORAGE_PREFIX}:${String(caseId || "").trim()}`;
}

function purgePersistedStatsFlowContexts(caseId = ""): void {
  if (typeof window === "undefined") {
    return;
  }
  const normalizedCaseId = String(caseId || "").trim();
  try {
    if (normalizedCaseId) {
      window.localStorage.removeItem(storageKey(normalizedCaseId));
    }
    Object.keys(window.localStorage).forEach((key) => {
      if (key.startsWith(`${STATS_FLOW_CONTEXT_STORAGE_PREFIX}:`)) {
        window.localStorage.removeItem(key);
      }
    });
  } catch {
    // The P0 contract remains no-write even when legacy cleanup is unavailable.
  }
}

export function readPersistedStatsFlowContexts(caseId: string): PersistedStatsFlowContexts | null {
  purgePersistedStatsFlowContexts(caseId);
  return null;
}

export function writePersistedStatsFlowContexts(
  caseId: string,
  value: {
    updatedAt?: string;
    treeTab?: string;
    mode?: string;
    relationPayload?: Record<string, unknown> | null;
    flowPayload?: Record<string, unknown> | null;
  }
): void {
  void value;
  purgePersistedStatsFlowContexts(caseId);
}

export function clearPersistedStatsFlowContexts(caseId: string): void {
  purgePersistedStatsFlowContexts(caseId);
}
