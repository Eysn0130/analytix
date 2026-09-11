import type {
  EmbeddedFlowAnchorRect,
  EmbeddedFlowRuntime,
  EmbeddedFlowShellBridge,
  EmbeddedFlowShellContextMenuItem,
  EmbeddedFlowShellDetailAction,
  EmbeddedFlowShellDetailRow,
  EmbeddedFlowShellMounts,
  EmbeddedFlowShellState,
  EmbeddedFlowShellStylePopoverOption,
  EmbeddedFlowShellStylePopoverTab,
  FlowCanvasAdapter,
} from "./embedded-flow-runtime";

export type FlowRuntimeId = "embedded";

export interface FlowRuntimeDescriptor {
  id: FlowRuntimeId;
  label: string;
  adapterPath: string;
  isFallback: boolean;
}

interface FlowRuntimeModule {
  ensureFlowCanvasAdapter: (apiBaseUrl: string) => Promise<FlowCanvasAdapter>;
}

const DEFAULT_FLOW_RUNTIME_ID: FlowRuntimeId = "embedded";
let invalidRuntimeWarned = false;

export type FlowRuntimeAdapter = FlowCanvasAdapter;
export type FlowShellBridge = EmbeddedFlowShellBridge;
export type FlowShellMounts = EmbeddedFlowShellMounts;
export type FlowShellState = EmbeddedFlowShellState;
export type FlowAnchorRect = EmbeddedFlowAnchorRect;
export type FlowShellContextMenuItem = EmbeddedFlowShellContextMenuItem;
export type FlowShellStylePopoverOption = EmbeddedFlowShellStylePopoverOption;
export type FlowShellStylePopoverTab = EmbeddedFlowShellStylePopoverTab;
export type FlowShellDetailRow = EmbeddedFlowShellDetailRow;
export type FlowShellDetailAction = EmbeddedFlowShellDetailAction;

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

function normalizeFlowRuntimeId(value: unknown): FlowRuntimeId {
  const raw = text(value).toLowerCase();
  if (!raw || raw === DEFAULT_FLOW_RUNTIME_ID) {
    return DEFAULT_FLOW_RUNTIME_ID;
  }
  if (!invalidRuntimeWarned) {
    invalidRuntimeWarned = true;
    console.warn("[flow-runtime] event=unsupported_runtime_fallback");
  }
  return DEFAULT_FLOW_RUNTIME_ID;
}

export function resolveConfiguredFlowRuntimeId(): FlowRuntimeId {
  return normalizeFlowRuntimeId(import.meta.env.VITE_FLOW_RUNTIME_ID);
}

function toRuntimeDescriptor(runtimeId: FlowRuntimeId): FlowRuntimeDescriptor {
  return {
    id: runtimeId,
    label: "Embedded Canvas Runtime",
    adapterPath: "./embedded-flow-runtime",
    isFallback: false,
  };
}

export function getFlowRuntimeDescriptor(): FlowRuntimeDescriptor {
  return toRuntimeDescriptor(resolveConfiguredFlowRuntimeId());
}

async function loadFlowRuntimeModule(_runtimeId: FlowRuntimeId): Promise<FlowRuntimeModule> {
  return await import("./embedded-flow-runtime");
}

export async function preloadFlowRuntimeModule(runtimeId = resolveConfiguredFlowRuntimeId()): Promise<void> {
  const resolvedId = normalizeFlowRuntimeId(runtimeId);
  await loadFlowRuntimeModule(resolvedId);
}

export async function ensureFlowRuntimeAdapter(
  apiBaseUrl: string,
  runtimeId = resolveConfiguredFlowRuntimeId()
): Promise<FlowRuntimeAdapter> {
  const resolvedId = normalizeFlowRuntimeId(runtimeId);
  const runtimeModule = await loadFlowRuntimeModule(resolvedId);
  return runtimeModule.ensureFlowCanvasAdapter(apiBaseUrl);
}

export const ensureFlowCanvasAdapter = ensureFlowRuntimeAdapter;

export async function ensureEmbeddedFlowRuntime(apiBaseUrl: string): Promise<EmbeddedFlowRuntime> {
  return ensureFlowRuntimeAdapter(apiBaseUrl, "embedded");
}
