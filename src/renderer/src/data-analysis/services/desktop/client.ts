import type {
  DataAnalysisBackendRuntimeState,
  DataAnalysisBridge,
  DataAnalysisRuntimeInfo
} from "../../types/desktop";
import {
  activateRuntimeBaseUrls,
  buildWsBaseUrlFromApiBase,
  clearRuntimeBaseUrls,
  hasDesktopDataAnalysisBridge,
} from "../runtime-base";
import { invalidateSharedWsRuntimes } from "../ws/shared-runtime";
import { THEME_TOKENS, type ThemeName } from "../../theme/tokens";

type DesktopPickFilesOptions = Parameters<DataAnalysisBridge["pickFiles"]>[0];

export type DesktopWindowChromeOptions = {
  backgroundColor?: string;
  symbolColor?: string;
  height?: number;
};

export const DESKTOP_STARTUP_WINDOW_CHROME = Object.freeze({
  backgroundColor: "#1a1a2e",
  symbolColor: "#ffffff",
  height: 36,
});

const BACKEND_RUNTIME_READY_TTL_MS = 1_500;

let ensureBackendRuntimeInFlight: Promise<boolean> | null = null;
let backendRuntimeReadyUntil = 0;
let highestObservedBackendGeneration = 0;
let terminalBackendGeneration = 0;
let terminalBackendAuthority = false;
let activeBackendGeneration = 0;
let activeBackendLaunchIdentity = "";

const BACKEND_LAUNCH_ID_PATTERN = /^[A-Za-z0-9_-]{43}$/u;

function getDesktopBridge(): DataAnalysisBridge | null {
  if (!hasDesktopDataAnalysisBridge()) {
    return null;
  }
  return window.analytix?.dataAnalysis ?? null;
}

function trimDesktopRuntimeUrl(value: unknown): string {
  return String(value == null ? "" : value).trim().replace(/\/+$/u, "");
}

function resolveDesktopRuntimeEndpoints(
  apiBaseValue: unknown,
  wsBaseValue: unknown,
): { apiBase: string; wsBase: string } | null {
  const apiBase = trimDesktopRuntimeUrl(apiBaseValue);
  try {
    const wsBase = trimDesktopRuntimeUrl(wsBaseValue) || buildWsBaseUrlFromApiBase(apiBase);
    const api = new URL(apiBase);
    const ws = new URL(wsBase);
    if (
      api.protocol !== "http:" ||
      api.hostname !== "127.0.0.1" ||
      !/^\d{1,5}$/u.test(api.port) ||
      Number(api.port) <= 0 ||
      Number(api.port) > 65535 ||
      api.username ||
      api.password ||
      (api.pathname !== "/" && api.pathname !== "") ||
      api.search ||
      api.hash ||
      ws.protocol !== "ws:" ||
      ws.hostname !== api.hostname ||
      ws.port !== api.port ||
      ws.username ||
      ws.password ||
      ws.pathname !== "/ws/events" ||
      ws.search ||
      ws.hash
    ) {
      return null;
    }
    return { apiBase, wsBase };
  } catch {
    return null;
  }
}

function clearDesktopRuntimeAuthority(): void {
  backendRuntimeReadyUntil = 0;
  activeBackendGeneration = 0;
  activeBackendLaunchIdentity = "";
  clearRuntimeBaseUrls();
  invalidateSharedWsRuntimes();
}

function normalizePathResult(value: unknown): string {
  if (typeof value === "string") {
    return value.trim();
  }
  if (value && typeof value === "object") {
    const record = value as { canceled?: boolean; path?: unknown; filePath?: unknown };
    if (record.canceled) {
      return "";
    }
    return String(record.path ?? record.filePath ?? "").trim();
  }
  return "";
}

function normalizeBooleanResult(value: unknown): boolean {
  if (typeof value === "boolean") {
    return value;
  }
  if (value && typeof value === "object" && "ok" in value) {
    return Boolean((value as { ok?: unknown }).ok);
  }
  return false;
}

export function hasDesktopBridge(): boolean {
  return hasDesktopDataAnalysisBridge();
}

export async function getDesktopRuntimeInfo(): Promise<DataAnalysisRuntimeInfo | null> {
  const desktop = getDesktopBridge();
  if (!desktop) {
    return null;
  }
  try {
    return await desktop.getRuntimeInfo();
  } catch {
    return null;
  }
}

export function applyDesktopBackendRuntimeState(state: DataAnalysisBackendRuntimeState | null | undefined): boolean {
  if (
    state?.terminal ||
    state?.authority === "unavailable" ||
    state?.blocker === "data_analysis_native_authority_unavailable"
  ) {
    terminalBackendAuthority = true;
    clearDesktopRuntimeAuthority();
    return false;
  }
  const generation = Number(state?.generation || 0);
  const hasGeneration = Number.isSafeInteger(generation) && generation > 0;
  if (hasGeneration && generation < highestObservedBackendGeneration) {
    return false;
  }
  if (hasGeneration) {
    highestObservedBackendGeneration = Math.max(highestObservedBackendGeneration, generation);
  }
  if (!state || state.phase !== "running") {
    if (hasGeneration && (state?.phase === "stopped" || state?.phase === "failed")) {
      terminalBackendGeneration = Math.max(terminalBackendGeneration, generation);
    }
    clearDesktopRuntimeAuthority();
    return false;
  }
  const launchId = String(state.launchId || "").trim();
  const endpoints = resolveDesktopRuntimeEndpoints(state.apiBase, state.wsBase);
  if (
    !hasGeneration ||
    generation <= terminalBackendGeneration ||
    !BACKEND_LAUNCH_ID_PATTERN.test(launchId) ||
    !endpoints
  ) {
    if (hasGeneration) {
      terminalBackendGeneration = Math.max(terminalBackendGeneration, generation);
    }
    clearDesktopRuntimeAuthority();
    return false;
  }
  const launchIdentity = `${generation}:${launchId}`;
  if (
    activeBackendGeneration === generation &&
    activeBackendLaunchIdentity &&
    activeBackendLaunchIdentity !== launchIdentity
  ) {
    terminalBackendGeneration = Math.max(terminalBackendGeneration, generation);
    clearDesktopRuntimeAuthority();
    return false;
  }
  activateRuntimeBaseUrls(endpoints.apiBase, endpoints.wsBase, generation, launchIdentity);
  activeBackendGeneration = generation;
  activeBackendLaunchIdentity = launchIdentity;
  return true;
}

export async function syncDesktopBackendRuntimeBase(): Promise<boolean> {
  const runtimeInfo = await getDesktopRuntimeInfo();
  return applyDesktopBackendRuntimeState(runtimeInfo?.backend);
}

export async function ensureDesktopBackendRuntime(): Promise<boolean> {
  const desktop = getDesktopBridge();
  if (terminalBackendAuthority || !desktop || typeof desktop.ensureBackend !== "function") {
    return false;
  }
  if (backendRuntimeReadyUntil > Date.now()) {
    return true;
  }
  if (ensureBackendRuntimeInFlight) {
    return ensureBackendRuntimeInFlight;
  }
  const request = (async (): Promise<boolean> => {
    try {
      const state = await desktop.ensureBackend();
      const ready = applyDesktopBackendRuntimeState(state);
      if (ready) {
        backendRuntimeReadyUntil = Date.now() + BACKEND_RUNTIME_READY_TTL_MS;
      }
      return ready;
    } catch {
      return false;
    }
  })();
  ensureBackendRuntimeInFlight = request;
  const cleanup = (): void => {
    if (ensureBackendRuntimeInFlight === request) {
      ensureBackendRuntimeInFlight = null;
    }
  };
  request.then(cleanup, cleanup);
  return request;
}

export function clearDesktopBackendRuntimeCacheForTests(): void {
  ensureBackendRuntimeInFlight = null;
  backendRuntimeReadyUntil = 0;
  highestObservedBackendGeneration = 0;
  terminalBackendGeneration = 0;
  terminalBackendAuthority = false;
  clearDesktopRuntimeAuthority();
}

export async function forceEnsureDesktopBackendRuntime(): Promise<boolean> {
  backendRuntimeReadyUntil = 0;
  try {
    return await ensureDesktopBackendRuntime();
  } catch {
    return false;
  }
}

export function subscribeDesktopBackendRuntimeState(
  callback?: (state: DataAnalysisBackendRuntimeState, ready: boolean) => void
): () => void {
  const desktop = getDesktopBridge();
  if (!desktop || typeof desktop.onBackendRuntimeState !== "function") {
    return () => {};
  }
  return desktop.onBackendRuntimeState((state) => {
    const ready = applyDesktopBackendRuntimeState(state);
    callback?.(state, ready);
  });
}

export async function setDesktopWindowChrome(options: DesktopWindowChromeOptions = {}): Promise<boolean> {
  const desktop = getDesktopBridge();
  if (!desktop || typeof desktop.setWindowChrome !== "function") {
    return false;
  }
  try {
    return normalizeBooleanResult(await desktop.setWindowChrome(options));
  } catch {
    return false;
  }
}

export async function updateDesktopWindowChrome(theme: ThemeName): Promise<boolean> {
  const palette = THEME_TOKENS[theme] ?? THEME_TOKENS.light;
  return setDesktopWindowChrome({
    backgroundColor: palette["sidebar-surface"] || palette["color-bg"] || "#edf1f8",
    symbolColor: palette["color-text"] || "#1a2740",
    height: 36
  });
}

export async function pickFiles(options?: DesktopPickFilesOptions): Promise<string[]> {
  const desktop = getDesktopBridge();
  if (!desktop) {
    return [];
  }
  try {
    const result = await desktop.pickFiles(options ?? {});
    if (Array.isArray(result)) {
      return result.map((item) => String(item || "").trim()).filter(Boolean);
    }
    if (result && typeof result === "object") {
      const record = result as { canceled?: boolean; paths?: unknown; filePaths?: unknown };
      if (record.canceled) {
        return [];
      }
      const paths = Array.isArray(record.paths) ? record.paths : Array.isArray(record.filePaths) ? record.filePaths : [];
      return paths.map((item) => String(item || "").trim()).filter(Boolean);
    }
  } catch {
    return [];
  }
  return [];
}

export async function pickDirectory(): Promise<string> {
  const desktop = getDesktopBridge();
  if (!desktop) {
    return "";
  }
  try {
    return normalizePathResult(await desktop.pickDirectory());
  } catch {
    return "";
  }
}

export async function openDesktopExternal(url: string): Promise<boolean> {
  const href = String(url || "").trim();
  if (!href) {
    return false;
  }
  const desktop = getDesktopBridge();
  if (!desktop || typeof desktop.openExternal !== "function") {
    return false;
  }
  try {
    return normalizeBooleanResult(await desktop.openExternal(href));
  } catch {
    return false;
  }
}
