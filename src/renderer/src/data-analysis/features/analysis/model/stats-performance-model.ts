export const STATS_PERFORMANCE_EVENT_NAME = "analytix:stats-performance";
export const STATS_PERFORMANCE_BUFFER_LIMIT = 80;

export type StatsPerformanceStage =
  | "rows.query"
  | "rows.apply"
  | "txn.open"
  | "txn.query"
  | "txn.apply"
  | "txn.scroll-load-trigger";

export interface StatsPerformanceMark {
  id: string;
  stage: StatsPerformanceStage;
  startedAt: number;
  meta: Record<string, unknown>;
}

export interface StatsPerformanceEntry {
  id: string;
  stage: StatsPerformanceStage;
  startedAt: number;
  endedAt: number;
  durationMs: number;
  meta: Record<string, unknown>;
}

export interface StatsPerformanceStore {
  events: StatsPerformanceEntry[];
  last: StatsPerformanceEntry | null;
}

type StatsPerformanceHost = typeof globalThis & {
  __ANALYTIX_STATS_PERFORMANCE__?: StatsPerformanceStore;
  CustomEvent?: typeof CustomEvent;
};

let nextStatsPerformanceId = 1;

export function readStatsPerformanceNow(): number {
  const perf = globalThis.performance;
  if (perf && typeof perf.now === "function") {
    return perf.now();
  }
  return Date.now();
}

export function startStatsPerformanceStage(
  stage: StatsPerformanceStage,
  meta: Record<string, unknown> = {},
  now: () => number = readStatsPerformanceNow
): StatsPerformanceMark {
  return {
    id: `stats-perf-${nextStatsPerformanceId++}`,
    stage,
    startedAt: normalizeTime(now()),
    meta: sanitizeStatsPerformanceMeta(meta),
  };
}

export function finishStatsPerformanceStage(
  mark: StatsPerformanceMark,
  meta: Record<string, unknown> = {},
  now: () => number = readStatsPerformanceNow
): StatsPerformanceEntry {
  const endedAt = normalizeTime(now());
  const startedAt = normalizeTime(mark.startedAt);
  return {
    id: mark.id,
    stage: mark.stage,
    startedAt,
    endedAt,
    durationMs: roundMs(Math.max(0, endedAt - startedAt)),
    meta: sanitizeStatsPerformanceMeta({ ...mark.meta, ...meta }),
  };
}

export function publishStatsPerformanceEntry(
  entry: StatsPerformanceEntry,
  options: {
    host?: StatsPerformanceHost;
    target?: EventTarget | null;
    bufferLimit?: number;
  } = {}
): StatsPerformanceEntry {
  const host = options.host || (globalThis as StatsPerformanceHost);
  const store = getStatsPerformanceStore(host);
  store.events.push(entry);
  const limit = Math.max(1, Math.floor(Number(options.bufferLimit ?? STATS_PERFORMANCE_BUFFER_LIMIT)));
  while (store.events.length > limit) {
    store.events.shift();
  }
  store.last = entry;

  const target = options.target === undefined ? defaultStatsPerformanceTarget() : options.target;
  const CustomEventCtor = host.CustomEvent || (typeof CustomEvent !== "undefined" ? CustomEvent : null);
  if (target && CustomEventCtor) {
    target.dispatchEvent(new CustomEventCtor(STATS_PERFORMANCE_EVENT_NAME, { detail: entry }));
  }
  return entry;
}

export function getStatsPerformanceStore(host: StatsPerformanceHost = globalThis as StatsPerformanceHost): StatsPerformanceStore {
  if (!host.__ANALYTIX_STATS_PERFORMANCE__) {
    host.__ANALYTIX_STATS_PERFORMANCE__ = {
      events: [],
      last: null,
    };
  }
  return host.__ANALYTIX_STATS_PERFORMANCE__;
}

function defaultStatsPerformanceTarget(): EventTarget | null {
  if (typeof window !== "undefined" && typeof window.dispatchEvent === "function") {
    return window;
  }
  return null;
}

function sanitizeStatsPerformanceMeta(meta: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  Object.entries(meta).forEach(([key, value]) => {
    if (value === undefined) {
      return;
    }
    out[key] = value;
  });
  return out;
}

function normalizeTime(value: number): number {
  return Number.isFinite(value) ? Number(value) : 0;
}

function roundMs(value: number): number {
  return Math.round(value * 1000) / 1000;
}
