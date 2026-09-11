import type {
  SafeWsChannel,
  SafeWsEventCounters,
  SafeWsEventEnvelope,
  SafeWsEventLevel,
  SafeWsEventName,
  SafeWsEventPayload,
  SafeWsEventStatus,
  SafeWsEventType,
  SafeWsExportTable,
} from "./types";

export const IMPORT_EVENT_NAMES = new Set([
  "import.job.queued",
  "import.job.progress",
  "import.job.completed",
  "import.job.failed"
]);

export const CLEANING_EVENT_NAMES = new Set([
  "cleaning.job.queued",
  "cleaning.job.progress",
  "cleaning.job.log",
  "cleaning.job.completed",
  "cleaning.job.failed"
]);

export const FLOW_BUILD_EVENT_NAMES = new Set([
  "analysis.flow.build.progress",
  "analysis.flow.build.completed",
  "analysis.flow.graph.patch",
  "analysis.flow.views.patch",
  "analysis.flow.build.failed"
]);

export const EXPORT_EVENT_NAMES = new Set([
  "export.job.queued",
  "export.job.progress",
  "export.job.completed",
  "export.job.failed"
]);

export const STATS_EXPORT_EVENT_NAMES = new Set([
  "analysis.stats.export.progress",
  "analysis.stats.export.completed",
  "analysis.stats.export.failed"
]);

export const STATS_QUERY_EVENT_NAMES = new Set([
  "analysis.stats.query.progress",
  "analysis.stats.query.completed",
  "analysis.stats.query.failed"
]);

export interface JobProgressPayload {
  progress?: number;
  stage?: string;
  status?: SafeWsEventStatus;
  message?: string;
  counters?: SafeWsEventCounters;
  code?: "JOB_CANCELED";
  event_id?: string;
}

export interface CleaningLogPayload {
  level?: "debug" | "info" | "warn" | "error";
  message?: string;
  step?: number;
  event_id?: string;
}

export interface FlowBuildCompletedPayload {
  node_count?: number;
  edge_count?: number;
  build_ms?: number;
  result_snapshot_ref?: Record<string, unknown> | null;
  trace_id?: string;
  event_id?: string;
}

export interface FlowGraphPatchPayload {
  base_snapshot_ref?: Record<string, unknown> | null;
  result_snapshot_ref?: Record<string, unknown> | null;
  base_runtime_revision?: number | null;
  runtime_revision?: number | null;
  graph_tier?: string;
  render_hints?: Record<string, unknown> | null;
  projection?: Record<string, unknown> | null;
  trace_id?: string;
  patch_kind?: "delta" | "full" | string;
  runtime_graph_patch?: Record<string, unknown> | null;
  runtime_graph?: Record<string, unknown> | null;
  nodes?: Array<Record<string, unknown>>;
  edges?: Array<Record<string, unknown>>;
  stats?: Record<string, unknown>;
  patch_scope?: "build" | "view-activate" | "layout-switch" | "local-update" | string;
  patch_source?: "ws" | "local" | string;
  view_id?: string;
  layout_preset?: string;
  reason?: string;
  event_id?: string;
}

export interface FlowViewsPatchPayload {
  operation?: "create" | "update" | "delete" | "reorder" | string;
  view?: Record<string, unknown> | null;
  view_id?: string;
  order?: string[];
  count?: number;
  event_id?: string;
}

export interface ExportProgressPayload {
  progress?: number;
  stage?: string;
  message?: string;
  output_path?: string;
  path?: string;
  counters?: SafeWsEventCounters;
  code?: string;
  details?: Record<string, unknown>;
  result_ref?: string;
  event_id?: string;
}

interface SafeEventMetadata {
  channel: SafeWsChannel;
  status: Exclude<SafeWsEventStatus, "canceled">;
}

const SAFE_EVENT_METADATA: Record<SafeWsEventName, SafeEventMetadata> = {
  "import.job.queued": { channel: "import", status: "queued" },
  "import.job.progress": { channel: "import", status: "running" },
  "import.job.completed": { channel: "import", status: "completed" },
  "import.job.failed": { channel: "import", status: "failed" },
  "cleaning.job.queued": { channel: "cleaning", status: "queued" },
  "cleaning.job.progress": { channel: "cleaning", status: "running" },
  "cleaning.job.log": { channel: "cleaning", status: "running" },
  "cleaning.job.completed": { channel: "cleaning", status: "completed" },
  "cleaning.job.failed": { channel: "cleaning", status: "failed" },
  "analysis.flow.build.progress": { channel: "analysis", status: "running" },
  "analysis.flow.build.completed": { channel: "analysis", status: "completed" },
  "analysis.flow.graph.patch": { channel: "analysis", status: "running" },
  "analysis.flow.views.patch": { channel: "analysis", status: "running" },
  "analysis.flow.build.failed": { channel: "analysis", status: "failed" },
  "export.job.queued": { channel: "export", status: "queued" },
  "export.job.progress": { channel: "export", status: "running" },
  "export.job.completed": { channel: "export", status: "completed" },
  "export.job.failed": { channel: "export", status: "failed" },
  "analysis.stats.export.progress": { channel: "analysis", status: "running" },
  "analysis.stats.export.completed": { channel: "analysis", status: "completed" },
  "analysis.stats.export.failed": { channel: "analysis", status: "failed" },
  "analysis.stats.query.progress": { channel: "analysis", status: "running" },
  "analysis.stats.query.completed": { channel: "analysis", status: "completed" },
  "analysis.stats.query.failed": { channel: "analysis", status: "failed" },
};

const SAFE_EVENT_TYPES = new Set<SafeWsEventType>([
  "info",
  "progress",
  "success",
  "warning",
  "error",
  "log",
]);
const SAFE_EXPORT_TABLES = new Set<SafeWsExportTable>([
  "fc_coercive_measure",
  "fc_person_contact",
  "fc_person_address",
  "fc_person",
  "fc_sub_account",
  "fc_account",
  "fc_transaction",
  "fc_task_fail",
  "fc_task_success",
]);
const SAFE_COUNTER_KEYS = [
  "rows_total",
  "rows_exported",
  "requested_steps",
  "cleaned_rows",
  "invalid",
  "failed",
  "reversal",
  "step",
  "job_count",
  "processed_count",
  "record_count",
  "row_count",
  "skipped_count",
] as const satisfies ReadonlyArray<Exclude<keyof SafeWsEventCounters, "table">>;
const SAFE_CASE_ID_PATTERN = /^[A-Za-z0-9_-]{4,80}$/u;
const CANONICAL_AUTHORITY_ENTITY_REF_PATTERN = /^cer1_[a-p]{64}$/u;
const CANONICAL_SOURCE_ROW_REF_PATTERN = /^srow1_[0-9a-f]{64}$/u;
const SAFE_JOB_ID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u;
const SAFE_TIMESTAMP_PATTERN = /^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?(?:Z|[+-][0-9]{2}:[0-9]{2})$/u;
const SAFE_EVENT_ID_PATTERN = /^[0-9]{1,16}:[a-z][a-z0-9.]{2,63}:[A-Za-z0-9_:-]{4,128}$/u;
const READ_MISSING = Symbol("read_missing");
const READ_BLOCKED = Symbol("read_blocked");

function readOwnDataProperty(value: unknown, key: string): unknown | typeof READ_MISSING | typeof READ_BLOCKED {
  if ((typeof value !== "object" && typeof value !== "function") || value === null) {
    return READ_MISSING;
  }
  try {
    const descriptor = Object.getOwnPropertyDescriptor(value, key);
    if (!descriptor) {
      return READ_MISSING;
    }
    if (!("value" in descriptor)) {
      return READ_BLOCKED;
    }
    return descriptor.value;
  } catch {
    return READ_BLOCKED;
  }
}

function safeString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

export function isCanonicalPrivateWsReference(value: string): boolean {
  return CANONICAL_AUTHORITY_ENTITY_REF_PATTERN.test(value)
    || CANONICAL_SOURCE_ROW_REF_PATTERN.test(value);
}

function safeBoundedInteger(value: unknown, minimum: number, maximum: number): number | null {
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    Object.is(value, -0) ||
    value < minimum ||
    value > maximum
  ) {
    return null;
  }
  return value;
}

function safeEventName(value: unknown): SafeWsEventName | null {
  const eventName = safeString(value) as SafeWsEventName;
  return Object.prototype.hasOwnProperty.call(SAFE_EVENT_METADATA, eventName) ? eventName : null;
}

function safeEventType(value: unknown): SafeWsEventType | null {
  const eventType = safeString(value) as SafeWsEventType;
  return SAFE_EVENT_TYPES.has(eventType) ? eventType : null;
}

function safeEventLevel(eventType: SafeWsEventType): SafeWsEventLevel {
  return eventType === "log" ? "info" : eventType;
}

function safeEventId(value: unknown): string {
  const eventId = safeString(value);
  return SAFE_EVENT_ID_PATTERN.test(eventId) ? eventId : "";
}

function projectSafeCounters(value: unknown): SafeWsEventCounters | undefined {
  if ((typeof value !== "object" && typeof value !== "function") || value === null) {
    return undefined;
  }
  const counters: Partial<Record<keyof SafeWsEventCounters, number | SafeWsExportTable>> = {};
  const table = safeString(readOwnDataProperty(value, "table")) as SafeWsExportTable;
  if (SAFE_EXPORT_TABLES.has(table)) {
    counters.table = table;
  }
  for (const key of SAFE_COUNTER_KEYS) {
    const count = safeBoundedInteger(readOwnDataProperty(value, key), 0, 1_000_000_000);
    if (count !== null) {
      counters[key] = count;
    }
  }
  if (Object.keys(counters).length === 0) {
    return undefined;
  }
  return Object.freeze(counters) as SafeWsEventCounters;
}

function projectSafePayload(
  value: unknown,
  eventName: SafeWsEventName,
  eventType: SafeWsEventType,
  status: Exclude<SafeWsEventStatus, "canceled">,
  sequence: number,
  identity: string,
): SafeWsEventPayload {
  const payload: {
    event_id: string;
    status: SafeWsEventStatus;
    stage: SafeWsEventStatus;
    level: SafeWsEventLevel;
    progress?: number;
    step?: number;
    code?: "JOB_CANCELED";
    counters?: SafeWsEventCounters;
  } = {
    event_id: `${sequence}:${eventName}:${identity}`,
    status,
    stage: status,
    level: safeEventLevel(eventType),
  };
  const progress = safeBoundedInteger(readOwnDataProperty(value, "progress"), 0, 100);
  if (progress !== null) {
    payload.progress = progress;
  }
  if (eventName.startsWith("cleaning.")) {
    const step = safeBoundedInteger(readOwnDataProperty(value, "step"), 1, 10);
    if (step !== null) {
      payload.step = step;
    }
  }
  if (eventName.endsWith(".failed")) {
    const code = safeString(readOwnDataProperty(value, "code"));
    if (code === "JOB_CANCELED") {
      payload.code = code;
      payload.status = "canceled";
    }
  }
  const counters = projectSafeCounters(readOwnDataProperty(value, "counters"));
  if (counters) {
    payload.counters = counters;
  }
  return Object.freeze(payload);
}

export function projectSafeWsEvent(value: unknown, expectedCaseId: string): SafeWsEventEnvelope | null {
  const eventName = safeEventName(readOwnDataProperty(value, "event"));
  if (!eventName) {
    return null;
  }
  const metadata = SAFE_EVENT_METADATA[eventName];
  if (safeString(readOwnDataProperty(value, "version")) !== "v1") {
    return null;
  }
  const eventType = safeEventType(readOwnDataProperty(value, "type"));
  if (!eventType) {
    return null;
  }
  if (safeString(readOwnDataProperty(value, "channel")) !== metadata.channel) {
    return null;
  }
  const normalizedExpectedCaseId = safeString(expectedCaseId);
  if (
    !SAFE_CASE_ID_PATTERN.test(normalizedExpectedCaseId)
    || isCanonicalPrivateWsReference(normalizedExpectedCaseId)
  ) {
    return null;
  }
  if (safeString(readOwnDataProperty(value, "case_id")) !== normalizedExpectedCaseId) {
    return null;
  }
  const sequence = safeBoundedInteger(readOwnDataProperty(value, "sequence"), 0, Number.MAX_SAFE_INTEGER);
  if (sequence === null) {
    return null;
  }
  const timestamp = safeString(readOwnDataProperty(value, "timestamp"));
  if (!SAFE_TIMESTAMP_PATTERN.test(timestamp)) {
    return null;
  }
  const rawJobId = safeString(readOwnDataProperty(value, "job_id"));
  const jobId = SAFE_JOB_ID_PATTERN.test(rawJobId) ? rawJobId : null;
  const identity = jobId ?? normalizedExpectedCaseId;
  const rawPayload = readOwnDataProperty(value, "payload");
  const payloadValue = rawPayload === READ_BLOCKED || rawPayload === READ_MISSING ? null : rawPayload;
  const payload = projectSafePayload(payloadValue, eventName, eventType, metadata.status, sequence, identity);
  return Object.freeze({
    version: "v1",
    event: eventName,
    type: eventType,
    channel: metadata.channel,
    job_id: jobId,
    case_id: normalizedExpectedCaseId,
    sequence,
    payload,
    timestamp,
  });
}

export function isImportEvent(event: unknown): boolean {
  return IMPORT_EVENT_NAMES.has(safeString(readOwnDataProperty(event, "event")));
}

export function isCleaningEvent(event: unknown): boolean {
  return CLEANING_EVENT_NAMES.has(safeString(readOwnDataProperty(event, "event")));
}

export function isFlowBuildEvent(event: unknown): boolean {
  return FLOW_BUILD_EVENT_NAMES.has(safeString(readOwnDataProperty(event, "event")));
}

export function isExportEvent(event: unknown): boolean {
  return EXPORT_EVENT_NAMES.has(safeString(readOwnDataProperty(event, "event")));
}

export function isStatsExportEvent(event: unknown): boolean {
  return STATS_EXPORT_EVENT_NAMES.has(safeString(readOwnDataProperty(event, "event")));
}

export function isStatsQueryEvent(event: unknown): boolean {
  return STATS_QUERY_EVENT_NAMES.has(safeString(readOwnDataProperty(event, "event")));
}

export function eventBelongsToCase(event: unknown, caseId: string): boolean {
  const targetCaseId = safeString(caseId);
  if (!targetCaseId) {
    return true;
  }
  return safeString(readOwnDataProperty(event, "case_id")) === targetCaseId;
}

export function asJobProgressPayload(payload: unknown): JobProgressPayload {
  const projected: JobProgressPayload = {};
  const progress = safeBoundedInteger(readOwnDataProperty(payload, "progress"), 0, 100);
  if (progress !== null) {
    projected.progress = progress;
  }
  const stage = safeString(readOwnDataProperty(payload, "stage"));
  if (["queued", "running", "completed", "failed", "canceled"].includes(stage)) {
    projected.stage = stage;
  }
  const status = safeString(readOwnDataProperty(payload, "status")) as SafeWsEventStatus;
  if (["queued", "running", "completed", "failed", "canceled"].includes(status)) {
    projected.status = status;
  }
  const code = safeString(readOwnDataProperty(payload, "code"));
  if (code === "JOB_CANCELED") {
    projected.code = code;
  }
  const eventId = safeEventId(readOwnDataProperty(payload, "event_id"));
  if (eventId) {
    projected.event_id = eventId;
  }
  const counters = projectSafeCounters(readOwnDataProperty(payload, "counters"));
  if (counters) {
    projected.counters = counters;
  }
  return projected;
}

export function asCleaningLogPayload(payload: unknown): CleaningLogPayload {
  const projected: CleaningLogPayload = {};
  const level = safeString(readOwnDataProperty(payload, "level"));
  if (["debug", "info", "warn", "error"].includes(level)) {
    projected.level = level as CleaningLogPayload["level"];
  }
  const step = safeBoundedInteger(readOwnDataProperty(payload, "step"), 1, 10);
  if (step !== null) {
    projected.step = step;
  }
  const eventId = safeEventId(readOwnDataProperty(payload, "event_id"));
  if (eventId) {
    projected.event_id = eventId;
  }
  return projected;
}

export function asFlowBuildCompletedPayload(payload: unknown): FlowBuildCompletedPayload {
  return payload as FlowBuildCompletedPayload;
}

export function asFlowGraphPatchPayload(payload: unknown): FlowGraphPatchPayload {
  return payload as FlowGraphPatchPayload;
}

export function asFlowViewsPatchPayload(payload: unknown): FlowViewsPatchPayload {
  return payload as FlowViewsPatchPayload;
}

export function asExportProgressPayload(payload: unknown): ExportProgressPayload {
  const projected = asJobProgressPayload(payload) as ExportProgressPayload;
  const code = safeString(readOwnDataProperty(payload, "code"));
  if (code === "JOB_CANCELED") {
    projected.code = code;
  }
  return projected;
}
