import { WsEventClient } from "./ws-client";
import { isCanonicalPrivateWsReference, projectSafeWsEvent } from "./domain-events";
import type { SafeWsEventEnvelope, WsConnectionStatus } from "./types";
import {
  isDataAnalysisRuntimeEndpointAuthorityCurrent,
  resolveDataAnalysisRuntimeEndpointAuthority,
  type DataAnalysisRuntimeEndpointAuthority,
} from "../runtime-base";

function trimTrailingSlash(value: string): string {
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

const CONTROL_EVENT_NAMES = new Set([
  "llm.runtime.control.response",
  "llm.runtime.session.updated",
]);
const CONTROL_TOKEN_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/u;
const PURE_NUMERIC_PRIVATE_ID_PATTERN = /^[0-9]{7,32}$/u;
const OWN_DATA_MISSING = Symbol("own_data_missing");
const OWN_DATA_BLOCKED = Symbol("own_data_blocked");

function readOwnDataProperty(
  value: unknown,
  key: string,
): unknown | typeof OWN_DATA_MISSING | typeof OWN_DATA_BLOCKED {
  if ((typeof value !== "object" && typeof value !== "function") || value === null) {
    return OWN_DATA_MISSING;
  }
  try {
    const descriptor = Object.getOwnPropertyDescriptor(value, key);
    if (!descriptor) {
      return OWN_DATA_MISSING;
    }
    return "value" in descriptor ? descriptor.value : OWN_DATA_BLOCKED;
  } catch {
    return OWN_DATA_BLOCKED;
  }
}

function ownText(value: unknown, key: string): string {
  const item = readOwnDataProperty(value, key);
  return typeof item === "string" ? item.trim() : "";
}

function ownRecord(value: unknown, key: string): object | null {
  const item = readOwnDataProperty(value, key);
  return (typeof item === "object" || typeof item === "function") && item !== null ? item : null;
}

function safeControlToken(value: unknown): string {
  const normalized = typeof value === "string" ? value.trim() : "";
  return CONTROL_TOKEN_PATTERN.test(normalized) ? normalized : "";
}

function safePrivateFreeControlToken(value: unknown): string {
  const normalized = safeControlToken(value);
  if (
    !normalized
    || isCanonicalPrivateWsReference(normalized)
    || PURE_NUMERIC_PRIVATE_ID_PATTERN.test(normalized)
  ) {
    return "";
  }
  return normalized;
}

function safeControlRequestId(value: unknown): string {
  return safePrivateFreeControlToken(value);
}

function safeRuntimeSessionId(value: unknown): string {
  return safePrivateFreeControlToken(value);
}

function isControlSuccessEventType(value: unknown): boolean {
  const eventType = typeof value === "string" ? value.trim() : "";
  return eventType === "info" || eventType === "success";
}

function safeCheckpointVersion(value: unknown): number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
    ? value
    : 0;
}

function hasClosedRuntimeStatusMarker(value: unknown): boolean {
  if (typeof value === "string") {
    return value.trim().length > 0;
  }
  return (typeof value === "object" || typeof value === "function") && value !== null;
}

function projectRuntimeSessionStatus(value: unknown): Record<string, unknown> | null {
  if ((typeof value !== "object" && typeof value !== "function") || value === null) {
    return null;
  }
  const nested = ownRecord(value, "session_status");
  const candidate = nested ?? value;
  const status = readOwnDataProperty(candidate, "status");
  const thread = readOwnDataProperty(candidate, "thread");
  const session = readOwnDataProperty(candidate, "session");
  const replay = ownRecord(candidate, "replay");
  const runtimeState = readOwnDataProperty(candidate, "runtime_state");
  const sessionId = safeRuntimeSessionId(readOwnDataProperty(candidate, "session_id"));
  if (
    !sessionId ||
    !hasClosedRuntimeStatusMarker(status) ||
    !hasClosedRuntimeStatusMarker(thread) ||
    !hasClosedRuntimeStatusMarker(session) ||
    !replay ||
    !hasClosedRuntimeStatusMarker(runtimeState)
  ) {
    return null;
  }
  return Object.freeze({
    contract: "SafeRuntimeSessionStatusV1",
    session_id: sessionId,
    status: "available",
    thread: Object.freeze({ available: true }),
    session: Object.freeze({ available: true }),
    replay: Object.freeze({
      checkpoint_version: safeCheckpointVersion(readOwnDataProperty(replay, "checkpoint_version")),
    }),
    runtime_state: Object.freeze({ available: true }),
  });
}

function resolveCheckpointVersion(status: unknown): number {
  const projected = projectRuntimeSessionStatus(status);
  const replay = projected ? ownRecord(projected, "replay") : null;
  return replay ? safeCheckpointVersion(readOwnDataProperty(replay, "checkpoint_version")) : 0;
}

export function resolveWsEventsBaseUrl(apiBaseUrl?: string): string {
  return resolveDataAnalysisRuntimeEndpointAuthority({
    requestedApiBaseUrl: text(apiBaseUrl),
  }).wsBaseUrl;
}

type SharedWsRuntimeOptions = {
  apiBaseUrl?: string;
  wsBaseUrl?: string;
  caseId?: string;
};

type RetainOptions = {
  caseId?: string;
};

type SharedWsRuntimeRegistryHost = typeof globalThis & {
  __ANALYTIX_SHARED_WS_RUNTIME_REGISTRY__?: Map<string, SharedWsRuntime>;
};

export interface SharedWsRuntimeSnapshot {
  baseUrl: string;
  caseId: string;
  status: WsConnectionStatus;
  statusChangedAt: number;
  retainCount: number;
  statusListenerCount: number;
  eventListenerCount: number;
  connectRequestedCount: number;
  openCount: number;
  closeCount: number;
  errorCount: number;
  eventCount: number;
  reconnectCount: number;
  lastEventName: string;
  lastEventAt: number;
  lastEventCaseId: string;
}

export interface SharedWsControlRequest {
  request_id: string;
  type: "llm.control_request";
  action:
    | "runtime_status"
    | "interrupt_session"
    | "turn_steer"
    | "thread_compact"
    | "approval_decision"
    | "interactive_response";
  case_id?: string;
  session_id?: string;
  expected_turn_id?: string;
  input?: Array<Record<string, unknown>>;
  input_items?: Array<Record<string, unknown>>;
  approval_id?: string;
  decision?: "approve" | "reject";
  note?: string;
  interactive_request_id?: string;
  response_payload?: Record<string, unknown>;
  transport?: Record<string, unknown>;
}

type RuntimeSessionEnrollment = {
  caseId: string;
  sessionId: string;
  retainCount: number;
  checkpointVersion: number;
  requestInFlight: boolean;
};

type PendingControlRequest = {
  resolve: (value: Record<string, unknown>) => void;
  reject: (error: Error) => void;
  timerId: number | null;
  expectedAction: SharedWsControlRequest["action"];
  expectedSessionId: string;
};

export class SharedWsRuntime {
  private readonly client: WsEventClient;
  private status: WsConnectionStatus = "idle";
  private lastEvent: SafeWsEventEnvelope | null = null;
  private retainCount = 0;
  private statusChangedAt = 0;
  private connectRequestedCount = 0;
  private openCount = 0;
  private closeCount = 0;
  private errorCount = 0;
  private eventCount = 0;
  private reconnectCount = 0;
  private lastEventAt = 0;
  private lastEventName = "";
  private lastEventCaseId = "";
  private hasCompletedInitialConnect = false;
  private invalidated = false;

  private readonly statusListeners = new Set<(status: WsConnectionStatus) => void>();
  private readonly eventListeners = new Set<(event: SafeWsEventEnvelope) => void>();
  private readonly pendingControlRequests = new Map<
    string,
    PendingControlRequest
  >();
  private readonly runtimeSessionStatusBySessionId = new Map<string, Record<string, unknown>>();
  private readonly runtimeSessionEnrollments = new Map<string, RuntimeSessionEnrollment>();

  constructor(
    private readonly wsBaseUrl: string,
    private readonly authority: DataAnalysisRuntimeEndpointAuthority,
    private readonly caseId: string,
  ) {
    if (!caseId || isCanonicalPrivateWsReference(caseId)) {
      throw new Error(caseId ? "runtime_control_case_binding_invalid" : "runtime_control_case_binding_missing");
    }
    this.client = new WsEventClient({
      baseUrl: wsBaseUrl,
      caseId,
      isConnectionAuthorized: () => this.hasCurrentAuthority(),
    });
    this.client.onStatus((status) => {
      if (this.invalidated) {
        return;
      }
      if (!this.hasCurrentAuthority()) {
        this.invalidate();
        return;
      }
      const previousStatus = this.status;
      this.status = status;
      this.statusChangedAt = Date.now();
      if (status === "open") {
        this.openCount += 1;
      } else if (status === "closed") {
        this.closeCount += 1;
      } else if (status === "error") {
        this.errorCount += 1;
      } else if (status === "connecting" && previousStatus !== "idle") {
        this.reconnectCount += 1;
      }
      if (status === "open") {
        const enrollmentReason = this.hasCompletedInitialConnect ? "reconnect" : "enroll";
        this.hasCompletedInitialConnect = true;
        void this.replayRuntimeSessionEnrollments(enrollmentReason);
      }
      if (status === "closed" || status === "error") {
        this.rejectPendingControlRequests(new Error("runtime_control_transport_closed"));
      }
      this.statusListeners.forEach((listener) => {
        try {
          listener(status);
        } catch {
          // Ignore consumer errors to keep the shared transport alive.
        }
      });
    });
    this.client.onEvent((event) => {
      if (this.invalidated) {
        return;
      }
      if (!this.hasCurrentAuthority()) {
        this.invalidate();
        return;
      }
      if (ownText(event, "case_id") !== this.caseId) {
        return;
      }
      const eventName = ownText(event, "event");
      if (CONTROL_EVENT_NAMES.has(eventName)) {
        const payload = ownRecord(event, "payload");
        if (!payload) {
          return;
        }
        if (eventName === "llm.runtime.control.response") {
          const requestId = safeControlRequestId(readOwnDataProperty(payload, "request_id"));
          const pending = requestId ? this.pendingControlRequests.get(requestId) : undefined;
          if (!pending) {
            return;
          }
          this.pendingControlRequests.delete(requestId);
          if (pending.timerId !== null) {
            window.clearTimeout(pending.timerId);
          }
          if (ownText(event, "type") === "error") {
            pending.reject(new Error("runtime_control_request_failed"));
            return;
          }
          if (!isControlSuccessEventType(readOwnDataProperty(event, "type"))) {
            pending.reject(new Error("runtime_control_response_invalid"));
            return;
          }
          if (pending.expectedAction !== "runtime_status") {
            pending.resolve(Object.freeze({}));
            return;
          }
          const responseData = readOwnDataProperty(payload, "data");
          const runtimeStatus = projectRuntimeSessionStatus(responseData);
          if (!runtimeStatus) {
            pending.reject(new Error("runtime_control_response_invalid"));
            return;
          }
          if (ownText(runtimeStatus, "session_id") !== pending.expectedSessionId) {
            pending.reject(new Error("runtime_control_response_identity_mismatch"));
            return;
          }
          if (!this.commitRuntimeSessionStatus(runtimeStatus)) {
            pending.reject(new Error("runtime_control_session_not_enrolled"));
            return;
          }
          pending.resolve(Object.freeze({ session_status: runtimeStatus }));
        } else {
          if (!isControlSuccessEventType(readOwnDataProperty(event, "type"))) {
            return;
          }
          const runtimeStatus = projectRuntimeSessionStatus(payload);
          if (runtimeStatus) {
            this.commitRuntimeSessionStatus(runtimeStatus);
          }
        }
        return;
      }
      const safeEvent = projectSafeWsEvent(event, this.caseId);
      if (!safeEvent) {
        return;
      }
      this.lastEvent = safeEvent;
      this.eventCount += 1;
      this.lastEventAt = Date.now();
      this.lastEventName = safeEvent.event;
      this.lastEventCaseId = safeEvent.case_id;
      this.eventListeners.forEach((listener) => {
        try {
          listener(safeEvent);
        } catch {
          // Ignore consumer errors to keep the shared transport alive.
        }
      });
    });
  }

  getBaseUrl(): string {
    return this.wsBaseUrl;
  }

  getCaseId(): string {
    return this.caseId;
  }

  getStatus(): WsConnectionStatus {
    return this.status;
  }

  getLastEvent(): SafeWsEventEnvelope | null {
    return this.lastEvent;
  }

  retain(options: RetainOptions = {}): () => void {
    if (this.invalidated || !this.hasCurrentAuthority()) {
      throw new Error("runtime_control_generation_invalidated");
    }
    const nextCaseId = text(options.caseId);
    if (!this.caseId) {
      throw new Error("runtime_control_case_binding_missing");
    }
    if (nextCaseId && nextCaseId !== this.caseId) {
      throw new Error("runtime_control_case_binding_mismatch");
    }
    this.retainCount += 1;
    if (this.retainCount === 1) {
      this.connectRequestedCount += 1;
      this.client.connect();
    }
    let released = false;
    return () => {
      if (released) {
        return;
      }
      released = true;
      this.retainCount = Math.max(0, this.retainCount - 1);
      if (this.retainCount === 0) {
        this.client.disconnect();
      }
    };
  }

  retainRuntimeSession(options: { caseId?: string; sessionId?: string; checkpointVersion?: number }): () => void {
    if (this.invalidated || !this.hasCurrentAuthority()) {
      throw new Error("runtime_control_generation_invalidated");
    }
    const rawSessionId = text(options.sessionId);
    if (!rawSessionId) {
      return () => undefined;
    }
    const sessionId = safeRuntimeSessionId(rawSessionId);
    if (!sessionId) {
      throw new Error("runtime_control_session_id_invalid");
    }
    const caseId = text(options.caseId) || this.caseId;
    if (!caseId || caseId !== this.caseId) {
      throw new Error(caseId ? "runtime_control_case_binding_mismatch" : "runtime_control_case_binding_missing");
    }
    const cachedStatus = this.runtimeSessionStatusBySessionId.get(sessionId);
    const existing = this.runtimeSessionEnrollments.get(sessionId);
    const enrollment: RuntimeSessionEnrollment = existing ?? {
      caseId,
      sessionId,
      retainCount: 0,
      checkpointVersion: resolveCheckpointVersion(cachedStatus),
      requestInFlight: false,
    };
    enrollment.caseId = caseId || enrollment.caseId;
    enrollment.checkpointVersion = Math.max(
      enrollment.checkpointVersion,
      resolveCheckpointVersion(cachedStatus),
      Math.max(0, Number(options.checkpointVersion || 0)),
    );
    enrollment.retainCount += 1;
    this.runtimeSessionEnrollments.set(sessionId, enrollment);
    if (this.status === "open") {
      void this.requestRuntimeSessionEnrollment(
        sessionId,
        this.runtimeSessionStatusBySessionId.has(sessionId) ? "reconnect" : "enroll",
      );
    }
    let released = false;
    return () => {
      if (released) {
        return;
      }
      released = true;
      const current = this.runtimeSessionEnrollments.get(sessionId);
      if (!current) {
        return;
      }
      current.retainCount = Math.max(0, current.retainCount - 1);
      if (current.retainCount === 0) {
        this.runtimeSessionEnrollments.delete(sessionId);
      }
    };
  }

  getRuntimeSessionStatus(sessionId: string): Record<string, unknown> | null {
    if (this.invalidated) {
      return null;
    }
    if (!this.hasCurrentAuthority()) {
      this.invalidate();
      return null;
    }
    const normalizedSessionId = safeRuntimeSessionId(sessionId);
    if (!normalizedSessionId) {
      return null;
    }
    return this.runtimeSessionStatusBySessionId.get(normalizedSessionId) ?? null;
  }

  setCaseId(caseId: string): void {
    if (this.invalidated || !this.hasCurrentAuthority()) {
      return;
    }
    const normalized = text(caseId);
    if (!normalized) {
      throw new Error("runtime_control_case_binding_missing");
    }
    if (normalized !== this.caseId) {
      throw new Error("runtime_control_case_binding_immutable");
    }
  }

  onStatus(listener: (status: WsConnectionStatus) => void, options: { emitCurrent?: boolean } = {}): () => void {
    this.statusListeners.add(listener);
    if (options.emitCurrent !== false) {
      try {
        listener(this.status);
      } catch {
        // noop
      }
    }
    return () => {
      this.statusListeners.delete(listener);
    };
  }

  onEvent(listener: (event: SafeWsEventEnvelope) => void, options: { emitLast?: boolean } = {}): () => void {
    this.eventListeners.add(listener);
    if (options.emitLast && this.lastEvent) {
      try {
        listener(this.lastEvent);
      } catch {
        // noop
      }
    }
    return () => {
      this.eventListeners.delete(listener);
    };
  }

  requestControl(request: SharedWsControlRequest, options: { timeoutMs?: number } = {}): Promise<Record<string, unknown>> {
    if (this.invalidated || !this.hasCurrentAuthority()) {
      return Promise.reject(new Error("runtime_control_generation_invalidated"));
    }
    const requestCaseId = text(request.case_id);
    if (!this.caseId) {
      return Promise.reject(new Error("runtime_control_case_binding_missing"));
    }
    if (requestCaseId && isCanonicalPrivateWsReference(requestCaseId)) {
      return Promise.reject(new Error("runtime_control_case_binding_invalid"));
    }
    if (requestCaseId && requestCaseId !== this.caseId) {
      return Promise.reject(new Error("runtime_control_case_binding_mismatch"));
    }
    const rawRequestId = text(request.request_id);
    if (!rawRequestId) {
      return Promise.reject(new Error("runtime_control_request_id_missing"));
    }
    const requestId = safeControlRequestId(rawRequestId);
    if (!requestId) {
      return Promise.reject(new Error("runtime_control_request_id_invalid"));
    }
    const rawSessionId = text(request.session_id);
    const expectedSessionId = rawSessionId ? safeRuntimeSessionId(rawSessionId) : "";
    if (rawSessionId && !expectedSessionId) {
      return Promise.reject(new Error("runtime_control_session_id_invalid"));
    }
    if (request.action === "runtime_status" && !expectedSessionId) {
      return Promise.reject(new Error("runtime_control_session_id_missing"));
    }
    if (this.status !== "open") {
      return Promise.reject(new Error("runtime_control_socket_not_open"));
    }
    if (this.pendingControlRequests.has(requestId)) {
      return Promise.reject(new Error("runtime_control_request_id_duplicate"));
    }
    return new Promise<Record<string, unknown>>((resolve, reject) => {
      const timeoutMs = Math.max(1000, Number(options.timeoutMs) || 8000);
      const pending: PendingControlRequest = {
        resolve,
        reject,
        timerId: null,
        expectedAction: request.action,
        expectedSessionId,
      };
      const timerId =
        typeof window === "undefined"
          ? null
          : window.setTimeout(() => {
              if (this.pendingControlRequests.get(requestId) !== pending) {
                return;
              }
              this.pendingControlRequests.delete(requestId);
              pending.reject(new Error("runtime_control_request_timeout"));
            }, timeoutMs);
      pending.timerId = timerId;
      Object.freeze(pending);
      this.pendingControlRequests.set(requestId, pending);
      const sent = this.client.send({
        ...request,
        request_id: requestId,
        case_id: this.caseId,
        ...(expectedSessionId ? { session_id: expectedSessionId } : {}),
      });
      if (!sent) {
        if (timerId !== null) {
          window.clearTimeout(timerId);
        }
        if (this.pendingControlRequests.get(requestId) === pending) {
          this.pendingControlRequests.delete(requestId);
        }
        pending.reject(new Error("runtime_control_send_failed"));
      }
    });
  }

  cancelControlRequest(requestId: string): boolean {
    if (this.invalidated || !this.hasCurrentAuthority()) {
      return false;
    }
    const normalizedRequestId = safeControlRequestId(requestId);
    if (!normalizedRequestId) {
      return false;
    }
    return this.client.send({
      type: "llm.control_cancel",
      request_id: normalizedRequestId,
      case_id: this.caseId,
    });
  }

  getSnapshot(): SharedWsRuntimeSnapshot {
    return {
      baseUrl: this.wsBaseUrl,
      caseId: this.caseId,
      status: this.status,
      statusChangedAt: this.statusChangedAt,
      retainCount: this.retainCount,
      statusListenerCount: this.statusListeners.size,
      eventListenerCount: this.eventListeners.size,
      connectRequestedCount: this.connectRequestedCount,
      openCount: this.openCount,
      closeCount: this.closeCount,
      errorCount: this.errorCount,
      eventCount: this.eventCount,
      reconnectCount: this.reconnectCount,
      lastEventName: this.lastEventName,
      lastEventAt: this.lastEventAt,
      lastEventCaseId: this.lastEventCaseId
    };
  }

  invalidate(): void {
    if (this.invalidated) {
      return;
    }
    this.invalidated = true;
    this.retainCount = 0;
    this.client.disconnect();
    this.status = "closed";
    this.statusChangedAt = Date.now();
    this.rejectPendingControlRequests(new Error("runtime_control_generation_invalidated"));
    this.runtimeSessionEnrollments.clear();
    this.runtimeSessionStatusBySessionId.clear();
    this.lastEvent = null;
    this.lastEventName = "";
    this.lastEventAt = 0;
    this.lastEventCaseId = "";
    this.statusListeners.forEach((listener) => {
      try {
        listener("closed");
      } catch {
        // Ignore consumer errors while revoking the old transport generation.
      }
    });
    this.statusListeners.clear();
    this.eventListeners.clear();
  }

  private commitRuntimeSessionStatus(status: Record<string, unknown>): boolean {
    if (this.invalidated || !this.hasCurrentAuthority()) {
      return false;
    }
    const sessionId = safeRuntimeSessionId(readOwnDataProperty(status, "session_id"));
    const enrollment = this.runtimeSessionEnrollments.get(sessionId);
    if (!sessionId || !enrollment || enrollment.caseId !== this.caseId || enrollment.retainCount <= 0) {
      return false;
    }
    this.runtimeSessionStatusBySessionId.set(sessionId, status);
    enrollment.checkpointVersion = Math.max(enrollment.checkpointVersion, resolveCheckpointVersion(status));
    return true;
  }

  private rejectPendingControlRequests(error: Error): void {
    this.pendingControlRequests.forEach((pending) => {
      if (pending.timerId !== null) {
        window.clearTimeout(pending.timerId);
      }
      pending.reject(error);
    });
    this.pendingControlRequests.clear();
  }

  private async replayRuntimeSessionEnrollments(reason: "enroll" | "reconnect"): Promise<void> {
    if (this.invalidated || !this.hasCurrentAuthority()) {
      return;
    }
    for (const sessionId of this.runtimeSessionEnrollments.keys()) {
      await this.requestRuntimeSessionEnrollment(sessionId, reason);
    }
  }

  private async requestRuntimeSessionEnrollment(
    sessionId: string,
    reason: "enroll" | "reconnect",
  ): Promise<void> {
    const enrollment = this.runtimeSessionEnrollments.get(sessionId);
    if (
      this.invalidated ||
      !this.hasCurrentAuthority() ||
      !enrollment ||
      this.status !== "open" ||
      enrollment.requestInFlight
    ) {
      return;
    }
    enrollment.requestInFlight = true;
    try {
      const response = await this.requestControl(
        {
          type: "llm.control_request",
          request_id: `runtime-status-${reason}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
          action: "runtime_status",
          case_id: enrollment.caseId || this.caseId,
          session_id: enrollment.sessionId,
          transport: {
            reason,
            checkpoint_version: enrollment.checkpointVersion,
          },
        },
        { timeoutMs: 12000 },
      );
      const runtimeStatus = projectRuntimeSessionStatus(response);
      if (runtimeStatus) {
        this.commitRuntimeSessionStatus(runtimeStatus);
      }
    } catch {
      // Keep reconnect/enrollment best-effort; consumers will receive the next successful snapshot.
    } finally {
      enrollment.requestInFlight = false;
    }
  }

  private hasCurrentAuthority(): boolean {
    return isDataAnalysisRuntimeEndpointAuthorityCurrent(this.authority);
  }
}

function getSharedWsRuntimeRegistry(): Map<string, SharedWsRuntime> {
  const host = globalThis as SharedWsRuntimeRegistryHost;
  if (host.__ANALYTIX_SHARED_WS_RUNTIME_REGISTRY__ instanceof Map) {
    return host.__ANALYTIX_SHARED_WS_RUNTIME_REGISTRY__;
  }
  const registry = new Map<string, SharedWsRuntime>();
  host.__ANALYTIX_SHARED_WS_RUNTIME_REGISTRY__ = registry;
  return registry;
}

export function getSharedWsRuntime(options: SharedWsRuntimeOptions = {}): SharedWsRuntime {
  const authority = resolveDataAnalysisRuntimeEndpointAuthority({
    requestedApiBaseUrl: text(options.apiBaseUrl),
    requestedWsBaseUrl: trimTrailingSlash(text(options.wsBaseUrl)),
  });
  const resolvedBaseUrl = authority.wsBaseUrl;
  const caseId = text(options.caseId);
  if (!caseId) {
    throw new Error("runtime_control_case_binding_missing");
  }
  if (isCanonicalPrivateWsReference(caseId)) {
    throw new Error("runtime_control_case_binding_invalid");
  }
  const registryKey = JSON.stringify([authority.authorityId, resolvedBaseUrl, caseId]);
  const registry = getSharedWsRuntimeRegistry();
  const existing = registry.get(registryKey);
  if (existing) {
    return existing;
  }
  const runtime = new SharedWsRuntime(resolvedBaseUrl, authority, caseId);
  registry.set(registryKey, runtime);
  return runtime;
}

export function invalidateSharedWsRuntimes(): void {
  const registry = getSharedWsRuntimeRegistry();
  for (const runtime of registry.values()) {
    runtime.invalidate();
  }
  registry.clear();
}
