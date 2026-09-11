export interface StatsCacheInvalidationEvent {
  caseId: string;
  eventId: string;
  source: string;
  occurredAt: number;
}

type StatsCacheInvalidationListener = (event: StatsCacheInvalidationEvent) => void;

type StatsCacheInvalidationHost = typeof globalThis & {
  __ANALYTIX_STATS_CACHE_INVALIDATION_LISTENERS__?: Set<StatsCacheInvalidationListener>;
};

function getListenerStore(): Set<StatsCacheInvalidationListener> {
  const host = globalThis as StatsCacheInvalidationHost;
  if (host.__ANALYTIX_STATS_CACHE_INVALIDATION_LISTENERS__ instanceof Set) {
    return host.__ANALYTIX_STATS_CACHE_INVALIDATION_LISTENERS__;
  }
  const listeners = new Set<StatsCacheInvalidationListener>();
  host.__ANALYTIX_STATS_CACHE_INVALIDATION_LISTENERS__ = listeners;
  return listeners;
}

export function emitStatsCacheInvalidation(event: StatsCacheInvalidationEvent): void {
  const normalizedCaseId = String(event.caseId || "").trim();
  if (!normalizedCaseId) {
    return;
  }
  const listeners = getListenerStore();
  const payload: StatsCacheInvalidationEvent = {
    caseId: normalizedCaseId,
    eventId: String(event.eventId || `${event.source}:${normalizedCaseId}:${event.occurredAt}`).trim(),
    source: String(event.source || "unknown").trim() || "unknown",
    occurredAt: Number.isFinite(event.occurredAt) ? event.occurredAt : Date.now(),
  };
  listeners.forEach((listener) => {
    try {
      listener(payload);
    } catch {
      // Keep the invalidation bus best-effort and non-blocking.
    }
  });
}

export function subscribeStatsCacheInvalidation(listener: StatsCacheInvalidationListener): () => void {
  const listeners = getListenerStore();
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
