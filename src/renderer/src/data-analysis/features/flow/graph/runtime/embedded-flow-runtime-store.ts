import type { EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import type { EmbeddedFlowShellState } from "./embedded-flow-runtime";
import {
  createEmbeddedFlowRuntimeBackend,
  type EmbeddedFlowRuntimeBackend,
} from "./flow-runtime-backend";

type EmbeddedFlowRequestPayload = Record<string, unknown>;

interface FlowSignal<T> {
  connect: (listener: (value: T) => void) => void;
  disconnect: (listener: (value: T) => void) => void;
  emit: (value: T) => void;
}

export interface EmbeddedFlowRuntimeShellStateStore {
  getShellState: () => EmbeddedFlowShellState;
  publishShellState: (state: EmbeddedFlowShellState, meta?: { reason?: string }) => void;
  subscribeShellState: (
    listener: (state: EmbeddedFlowShellState, meta?: { reason?: string }) => void
  ) => () => void;
}

export interface EmbeddedFlowRuntimeBrowserBridge {
  __setCaseId: (caseId: string) => void;
  __openRequest: (payload: EmbeddedFlowRequestPayload) => void;
  __refit: () => void;
}

export interface EmbeddedFlowRuntimeStore extends EmbeddedFlowRuntimeShellStateStore {
  getBackend: () => EmbeddedFlowRuntimeBackend;
  getBrowserBridge: () => EmbeddedFlowRuntimeBrowserBridge;
  setCaseId: (caseId: string) => void;
  getCaseId: () => string;
  takePendingOpenRequest: () => EmbeddedFlowRequestPayload | null;
  queueOpenRequest: (payload: EmbeddedFlowRequestPayload) => void;
  consumePendingRefit: () => boolean;
  queueRefit: () => void;
}

interface CreateEmbeddedFlowRuntimeStoreOptions {
  service: EmbeddedFlowBridgeService;
  targetWindow: Window;
}

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

function cloneJson<T>(value: T, fallback: T): T {
  try {
    return JSON.parse(JSON.stringify(value)) as T;
  } catch {
    return fallback;
  }
}

function createSignal<T>(): FlowSignal<T> {
  const listeners = new Set<(value: T) => void>();
  return {
    connect(listener) {
      if (typeof listener === "function") listeners.add(listener);
    },
    disconnect(listener) {
      listeners.delete(listener);
    },
    emit(value) {
      listeners.forEach((listener) => {
        try {
          listener(value);
        } catch {
          // noop
        }
      });
    },
  };
}

export function createEmbeddedFlowRuntimeStore(options: CreateEmbeddedFlowRuntimeStoreOptions): EmbeddedFlowRuntimeStore {
  const { service, targetWindow } = options;
  let currentCaseId = "";
  let pendingOpenRequest: EmbeddedFlowRequestPayload | null = null;
  let pendingRefit = false;
  let latestShellState = {} as EmbeddedFlowShellState;
  const caseChanged = createSignal<string>();
  const shellListeners = new Set<(state: EmbeddedFlowShellState, meta?: { reason?: string }) => void>();

  const publishShellState = (state: EmbeddedFlowShellState, meta?: { reason?: string }) => {
    latestShellState = cloneJson(state, state);
    shellListeners.forEach((listener) => {
      try {
        listener(latestShellState, meta);
      } catch {
        // noop
      }
    });
  };

  const backend = createEmbeddedFlowRuntimeBackend({
    service,
    targetWindow,
    caseChanged,
    getCurrentCaseId: () => currentCaseId,
  });

  const browserBridge: EmbeddedFlowRuntimeBrowserBridge = {
    __setCaseId(caseId) {
      const nextCaseId = text(caseId);
      if (nextCaseId === currentCaseId) return;
      currentCaseId = nextCaseId;
      service.setCaseId(nextCaseId);
      caseChanged.emit(nextCaseId);
    },
    __openRequest(payload) {
      pendingOpenRequest = payload && typeof payload === "object" ? cloneJson(payload, {}) : {};
    },
    __refit() {
      pendingRefit = true;
    },
  };

  return {
    getBackend() {
      return backend;
    },
    getBrowserBridge() {
      return browserBridge;
    },
    setCaseId(caseId) {
      browserBridge.__setCaseId(caseId);
    },
    getCaseId() {
      return currentCaseId;
    },
    getShellState() {
      return cloneJson(latestShellState, latestShellState);
    },
    publishShellState,
    subscribeShellState(listener) {
      if (typeof listener !== "function") return () => undefined;
      shellListeners.add(listener);
      if (latestShellState && Object.keys(latestShellState).length) {
        try {
          listener(cloneJson(latestShellState, latestShellState), { reason: "subscribe" });
        } catch {
          // noop
        }
      }
      return () => {
        shellListeners.delete(listener);
      };
    },
    takePendingOpenRequest() {
      const payload = pendingOpenRequest ? cloneJson(pendingOpenRequest, {}) : null;
      pendingOpenRequest = null;
      return payload;
    },
    queueOpenRequest(payload) {
      browserBridge.__openRequest(payload);
    },
    consumePendingRefit() {
      const pending = pendingRefit;
      pendingRefit = false;
      return pending;
    },
    queueRefit() {
      browserBridge.__refit();
    },
  };
}
