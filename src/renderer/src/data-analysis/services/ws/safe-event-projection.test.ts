import { describe, expect, it } from "vitest";
import {
  applyCleaningRealtimeJobProjection,
  buildCleaningRealtimeProjection,
} from "../../features/cleaning/model/realtime-events";
import { applySafeWsEventToRuntimeState, type AppState } from "../../store/app-store";
import { projectSafeWsEvent } from "./domain-events";

const CASE_ID = "case-safe-ws";
const JOB_ID = "123e4567-e89b-42d3-a456-426614174000";
const ACCOUNT_CANARY = "6222020202020202020";
const PHONE_CANARY = "13800138000";
const AUTHORITY_REF_CANARY = `cer1_${"a".repeat(64)}`;
const SOURCE_ROW_REF_CANARY = `srow1_${"b".repeat(64)}`;
const LOCAL_PATH_CANARY = "/Users/synthetic/private/funds.duckdb";
const RAW_ROW_CANARY = `raw-row:${ACCOUNT_CANARY}:${PHONE_CANARY}`;
const HOSTILE_CANARIES = [
  ACCOUNT_CANARY,
  PHONE_CANARY,
  AUTHORITY_REF_CANARY,
  LOCAL_PATH_CANARY,
  RAW_ROW_CANARY,
];

function runtimeState(): AppState["runtime"] {
  return {
    backendHealth: "ok",
    wsStatus: "open",
    wsEventName: "",
    lastSyncAt: "",
    lastWsEvent: null,
  };
}

describe("safe renderer WebSocket event projection", () => {
  it("stores only a new closed projection and preserves safe progress fields", () => {
    const payloadDescriptorReads: string[] = [];
    const payloadValueReads: string[] = [];
    const countersDescriptorReads: string[] = [];
    let hostileGetterCalls = 0;

    const countersTarget = Object.create(null) as Record<string, unknown>;
    Object.defineProperties(countersTarget, {
      rows_total: { enumerable: true, value: 12 },
      rows_exported: { enumerable: true, value: 7 },
      table: { enumerable: true, value: "fc_transaction" },
      reverse_map: { enumerable: true, value: { [AUTHORITY_REF_CANARY]: ACCOUNT_CANARY } },
    });
    const counters = new Proxy(countersTarget, {
      getOwnPropertyDescriptor(target, property) {
        countersDescriptorReads.push(String(property));
        return Reflect.getOwnPropertyDescriptor(target, property);
      },
      ownKeys() {
        throw new Error("counter enumeration is forbidden");
      },
    });

    const payloadTarget = Object.create(null) as Record<string, unknown>;
    Object.defineProperties(payloadTarget, {
      progress: { enumerable: true, value: 42 },
      step: { enumerable: true, value: 3 },
      counters: { enumerable: true, value: counters },
      code: { enumerable: true, value: "NOT_ALLOWLISTED" },
      message: { enumerable: true, value: ACCOUNT_CANARY },
      path: { enumerable: true, value: LOCAL_PATH_CANARY },
      raw_rows: { enumerable: true, value: [{ account: ACCOUNT_CANARY, phone: PHONE_CANARY }] },
      details: {
        enumerable: true,
        get() {
          hostileGetterCalls += 1;
          throw new Error(RAW_ROW_CANARY);
        },
      },
      error: { enumerable: true, value: { body: RAW_ROW_CANARY } },
      authority_ref: { enumerable: true, value: AUTHORITY_REF_CANARY },
      toJSON: {
        enumerable: false,
        get() {
          hostileGetterCalls += 1;
          throw new Error("raw toJSON must not run");
        },
      },
    });
    const payload = new Proxy(payloadTarget, {
      getOwnPropertyDescriptor(target, property) {
        payloadDescriptorReads.push(String(property));
        return Reflect.getOwnPropertyDescriptor(target, property);
      },
      get(target, property, receiver) {
        payloadValueReads.push(String(property));
        return Reflect.get(target, property, receiver);
      },
      ownKeys() {
        throw new Error("payload enumeration is forbidden");
      },
    });

    const rawEvent = {
      version: "v1",
      event: "cleaning.job.progress",
      type: "progress",
      channel: "cleaning",
      job_id: JOB_ID,
      case_id: CASE_ID,
      sequence: 19,
      payload,
      timestamp: "2026-08-23T01:02:03.456Z",
      unknown_root: RAW_ROW_CANARY,
    };

    const safeEvent = projectSafeWsEvent(rawEvent, CASE_ID);
    expect(safeEvent).not.toBeNull();
    expect(safeEvent).not.toBe(rawEvent);
    expect(Object.is(safeEvent?.payload, payload)).toBe(false);
    expect(safeEvent).toMatchObject({
      version: "v1",
      event: "cleaning.job.progress",
      type: "progress",
      channel: "cleaning",
      job_id: JOB_ID,
      case_id: CASE_ID,
      sequence: 19,
      timestamp: "2026-08-23T01:02:03.456Z",
      payload: {
        event_id: `19:cleaning.job.progress:${JOB_ID}`,
        status: "running",
        stage: "running",
        level: "progress",
        progress: 42,
        step: 3,
        counters: {
          table: "fc_transaction",
          rows_total: 12,
          rows_exported: 7,
        },
      },
    });

    const nextRuntime = applySafeWsEventToRuntimeState(runtimeState(), safeEvent!);
    const serializedRuntime = JSON.stringify(nextRuntime);
    const cleaningProjection = buildCleaningRealtimeProjection(nextRuntime.lastWsEvent!);

    expect(nextRuntime.wsEventName).toBe("cleaning.job.progress");
    expect(nextRuntime.lastWsEvent).toBe(safeEvent);
    expect(cleaningProjection.liveEvent.progress).toBe(42);
    expect(cleaningProjection.liveEvent.step).toBe(3);
    expect(cleaningProjection.liveEvent.message).toBe("cleaning job progress");
    for (const canary of HOSTILE_CANARIES) {
      expect(serializedRuntime).not.toContain(canary);
      expect(JSON.stringify(cleaningProjection)).not.toContain(canary);
    }
    expect(hostileGetterCalls).toBe(0);
    expect(payloadValueReads).toEqual([]);
    expect(payloadDescriptorReads.sort()).toEqual(["counters", "progress", "step"]);
    expect(countersDescriptorReads).not.toContain("reverse_map");
  });

  it("withholds unknown events and types before reading their payload", () => {
    let payloadGetterCalls = 0;
    const base = {
      version: "v1",
      event: "unknown.private.event",
      type: "info",
      channel: "system",
      case_id: CASE_ID,
      sequence: 1,
      timestamp: "2026-08-23T01:02:03Z",
      get payload() {
        payloadGetterCalls += 1;
        throw new Error(ACCOUNT_CANARY);
      },
    };

    expect(projectSafeWsEvent(base, CASE_ID)).toBeNull();
    expect(payloadGetterCalls).toBe(0);

    const unknownType = {
      version: "v1",
      event: "import.job.progress",
      type: "trace",
      channel: "import",
      case_id: CASE_ID,
      sequence: 2,
      timestamp: "2026-08-23T01:02:04Z",
      get payload() {
        payloadGetterCalls += 1;
        throw new Error(PHONE_CANARY);
      },
    };
    expect(projectSafeWsEvent(unknownType, CASE_ID)).toBeNull();
    expect(payloadGetterCalls).toBe(0);
  });

  it("rejects canonical private references even when both case identities match", () => {
    for (const privateCaseId of [AUTHORITY_REF_CANARY, SOURCE_ROW_REF_CANARY]) {
      const projected = projectSafeWsEvent({
        version: "v1",
        event: "import.job.progress",
        type: "progress",
        channel: "import",
        job_id: JOB_ID,
        case_id: privateCaseId,
        sequence: 3,
        payload: { progress: 10 },
        timestamp: "2026-08-23T01:02:04Z",
      }, privateCaseId);
      const nextRuntime = projected
        ? applySafeWsEventToRuntimeState(runtimeState(), projected)
        : runtimeState();

      expect(projected).toBeNull();
      expect(nextRuntime.lastWsEvent).toBeNull();
      expect(JSON.stringify(nextRuntime)).not.toContain(privateCaseId);
    }
  });

  it("preserves cleaning completion refresh semantics without retaining a hostile job identity", () => {
    const safeEvent = projectSafeWsEvent({
      version: "v1",
      event: "cleaning.job.completed",
      type: "success",
      channel: "cleaning",
      job_id: AUTHORITY_REF_CANARY,
      case_id: CASE_ID,
      sequence: 21,
      payload: {
        progress: 100,
        message: PHONE_CANARY,
      },
      timestamp: "2026-08-23T01:02:05Z",
    }, CASE_ID);

    expect(safeEvent?.job_id).toBeNull();
    expect(JSON.stringify(safeEvent)).not.toContain(AUTHORITY_REF_CANARY);
    expect(JSON.stringify(safeEvent)).not.toContain(PHONE_CANARY);

    const update = buildCleaningRealtimeProjection(safeEvent!);
    expect(update.eventName).toBe("cleaning.job.completed");
    expect(update.liveEvent.progress).toBe(100);
    expect(update.liveEvent.message).toBe("cleaning completed");
    expect(update.shouldRefreshJobs).toBe(true);
    expect(update.shouldRefreshContext).toBe(true);
  });

  it("preserves only the closed cancellation code and maps a failed event to canceled", () => {
    const safeEvent = projectSafeWsEvent({
      version: "v1",
      event: "cleaning.job.failed",
      type: "error",
      channel: "cleaning",
      job_id: JOB_ID,
      case_id: CASE_ID,
      sequence: 22,
      payload: {
        code: "JOB_CANCELED",
        message: ACCOUNT_CANARY,
        error: PHONE_CANARY,
        path: LOCAL_PATH_CANARY,
        details: { authority_ref: AUTHORITY_REF_CANARY },
      },
      timestamp: "2026-08-23T01:02:06Z",
    }, CASE_ID);

    expect(safeEvent?.payload).toMatchObject({
      code: "JOB_CANCELED",
      status: "canceled",
      stage: "failed",
    });
    const update = buildCleaningRealtimeProjection(safeEvent!);
    const [job] = applyCleaningRealtimeJobProjection([{
      job_id: JOB_ID,
      case_id: CASE_ID,
      status: "running",
      progress: 52,
      result_semantic_status: "blocked",
      result_blocker: "host_evidence_receipt_required",
      fact_answer_allowed: false,
      cleaned_rows: null,
      summary: {},
      error: null,
      created_at: "2026-08-23T01:00:00Z",
      updated_at: "2026-08-23T01:01:00Z",
    }], update);
    const serialized = JSON.stringify({ safeEvent, update, job });

    expect(update.payload).toMatchObject({
      code: "JOB_CANCELED",
      status: "canceled",
    });
    expect(update.liveEvent.message).toBe("cleaning canceled");
    expect(job?.status).toBe("canceled");
    expect(update.shouldRefreshJobs).toBe(true);
    expect(update.shouldRefreshContext).toBe(true);
    for (const canary of HOSTILE_CANARIES) {
      expect(serialized).not.toContain(canary);
    }
  });

  it("does not invoke throwing accessors or proxy enumeration hooks", () => {
    let progressGetterCalls = 0;
    const payload = Object.create(null) as Record<string, unknown>;
    Object.defineProperties(payload, {
      progress: {
        enumerable: true,
        get() {
          progressGetterCalls += 1;
          throw new Error(PHONE_CANARY);
        },
      },
      step: { enumerable: true, value: 2 },
    });

    const safeEvent = projectSafeWsEvent({
      version: "v1",
      event: "cleaning.job.progress",
      type: "progress",
      channel: "cleaning",
      job_id: JOB_ID,
      case_id: CASE_ID,
      sequence: 20,
      payload,
      timestamp: "2026-08-23T01:02:04Z",
    }, CASE_ID);

    expect(safeEvent?.payload.progress).toBeUndefined();
    expect(safeEvent?.payload.step).toBe(2);
    expect(progressGetterCalls).toBe(0);

    const hostileRoot = new Proxy({}, {
      getOwnPropertyDescriptor() {
        throw new Error(AUTHORITY_REF_CANARY);
      },
      ownKeys() {
        throw new Error(LOCAL_PATH_CANARY);
      },
    });
    expect(() => projectSafeWsEvent(hostileRoot, CASE_ID)).not.toThrow();
    expect(projectSafeWsEvent(hostileRoot, CASE_ID)).toBeNull();
  });
});
