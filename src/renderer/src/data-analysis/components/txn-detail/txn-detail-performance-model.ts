export const TXN_DETAIL_PERFORMANCE_EVENT_NAME = "analytix:txn-detail-performance";
export const TXN_DETAIL_PERFORMANCE_BUFFER_LIMIT = 120;

export type TxnDetailPerformanceStage = "open" | "query" | "apply" | "scroll-load-trigger";

export interface TxnDetailPerformanceMark {
  id: string;
  operation: string;
  stage: TxnDetailPerformanceStage;
  startedAt: number;
  surface: string;
  meta: Record<string, unknown>;
}

export interface TxnDetailPerformanceEntry {
  id: string;
  operation: string;
  stage: TxnDetailPerformanceStage;
  startedAt: number;
  endedAt: number;
  durationMs: number;
  surface: string;
  meta: Record<string, unknown>;
}

export interface TxnDetailPerformanceStore {
  events: TxnDetailPerformanceEntry[];
  last: TxnDetailPerformanceEntry | null;
}

type TxnDetailPerformanceHost = typeof globalThis & {
  __ANALYTIX_TXN_DETAIL_PERFORMANCE__?: TxnDetailPerformanceStore;
  CustomEvent?: typeof CustomEvent;
};

let nextTxnDetailPerformanceId = 1;

export function readTxnDetailPerformanceNow(): number {
  const perf = globalThis.performance;
  if (perf && typeof perf.now === "function") {
    return perf.now();
  }
  return Date.now();
}

export function startTxnDetailPerformanceStage({
  meta = {},
  operation,
  stage,
  surface,
  now = readTxnDetailPerformanceNow
}: {
  meta?: Record<string, unknown>;
  operation: string;
  stage: TxnDetailPerformanceStage;
  surface: string;
  now?: () => number;
}): TxnDetailPerformanceMark {
  return {
    id: `txn-detail-perf-${nextTxnDetailPerformanceId++}`,
    operation: text(operation) || "detail",
    stage,
    startedAt: normalizeTime(now()),
    surface: text(surface) || "unknown",
    meta: sanitizeTxnDetailPerformanceMeta(meta)
  };
}

export function finishTxnDetailPerformanceStage(
  mark: TxnDetailPerformanceMark,
  meta: Record<string, unknown> = {},
  now: () => number = readTxnDetailPerformanceNow
): TxnDetailPerformanceEntry {
  const endedAt = normalizeTime(now());
  const startedAt = normalizeTime(mark.startedAt);
  return {
    id: mark.id,
    operation: mark.operation,
    stage: mark.stage,
    startedAt,
    endedAt,
    durationMs: roundMs(Math.max(0, endedAt - startedAt)),
    surface: mark.surface,
    meta: sanitizeTxnDetailPerformanceMeta({ ...mark.meta, ...meta })
  };
}

export function publishTxnDetailPerformanceEntry(
  entry: TxnDetailPerformanceEntry,
  options: {
    bufferLimit?: number;
    host?: TxnDetailPerformanceHost;
    target?: EventTarget | null;
  } = {}
): TxnDetailPerformanceEntry {
  const host = options.host || (globalThis as TxnDetailPerformanceHost);
  const store = getTxnDetailPerformanceStore(host);
  store.events.push(entry);
  const limit = Math.max(1, Math.floor(Number(options.bufferLimit ?? TXN_DETAIL_PERFORMANCE_BUFFER_LIMIT)));
  while (store.events.length > limit) {
    store.events.shift();
  }
  store.last = entry;

  const target = options.target === undefined ? defaultTxnDetailPerformanceTarget() : options.target;
  const CustomEventCtor = host.CustomEvent || (typeof CustomEvent !== "undefined" ? CustomEvent : null);
  if (target && CustomEventCtor) {
    target.dispatchEvent(new CustomEventCtor(TXN_DETAIL_PERFORMANCE_EVENT_NAME, { detail: entry }));
  }
  return entry;
}

export function getTxnDetailPerformanceStore(
  host: TxnDetailPerformanceHost = globalThis as TxnDetailPerformanceHost
): TxnDetailPerformanceStore {
  if (!host.__ANALYTIX_TXN_DETAIL_PERFORMANCE__) {
    host.__ANALYTIX_TXN_DETAIL_PERFORMANCE__ = {
      events: [],
      last: null
    };
  }
  return host.__ANALYTIX_TXN_DETAIL_PERFORMANCE__;
}

function defaultTxnDetailPerformanceTarget(): EventTarget | null {
  if (typeof window !== "undefined" && typeof window.dispatchEvent === "function") {
    return window;
  }
  return null;
}

function sanitizeTxnDetailPerformanceMeta(meta: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  Object.entries(meta).forEach(([key, value]) => {
    if (value !== undefined) {
      out[key] = value;
    }
  });
  return out;
}

function normalizeTime(value: number): number {
  return Number.isFinite(value) ? Number(value) : 0;
}

function roundMs(value: number): number {
  return Math.round(value * 1000) / 1000;
}

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}
