function trimTrailingSlash(value: string): string {
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

let runtimeApiBaseOverride = "";
let runtimeWsBaseOverride = "";
let activeRuntimeGeneration = 0;
let activeRuntimeAuthorityId = "";
let runtimeAbortController: AbortController | null = null;

export interface DataAnalysisRuntimeBaseLease {
  apiBaseUrl: string;
  wsBaseUrl: string;
  generation: number;
  authorityId: string;
  signal: AbortSignal;
}

export interface DataAnalysisRuntimeEndpointAuthority {
  kind: "desktop_lease";
  apiBaseUrl: string;
  wsBaseUrl: string;
  authorityId: string;
  lease: DataAnalysisRuntimeBaseLease;
}

export interface ResolveDataAnalysisRuntimeEndpointAuthorityOptions {
  requestedApiBaseUrl?: string;
  requestedWsBaseUrl?: string;
}

function normalizeHttpBase(value: unknown): string | null {
  const candidate = text(value);
  if (!candidate) {
    return null;
  }
  try {
    const url = new URL(candidate);
    if (
      (url.protocol !== "http:" && url.protocol !== "https:") ||
      url.username ||
      url.password ||
      (url.pathname !== "/" && url.pathname !== "") ||
      url.search ||
      url.hash
    ) {
      return null;
    }
    return trimTrailingSlash(url.toString());
  } catch {
    return null;
  }
}

function normalizeWsBase(value: unknown, apiBaseUrl: string): string | null {
  const candidate = text(value);
  if (!candidate) {
    return null;
  }
  try {
    const api = new URL(`${apiBaseUrl}/`);
    const ws = new URL(candidate);
    const expectedProtocol = api.protocol === "https:" ? "wss:" : "ws:";
    if (
      ws.protocol !== expectedProtocol ||
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
    return trimTrailingSlash(ws.toString());
  } catch {
    return null;
  }
}

function requireHttpBase(value: unknown): string {
  const normalized = normalizeHttpBase(value);
  if (!normalized) {
    throw new Error("data_analysis_backend_api_endpoint_invalid");
  }
  return normalized;
}

function requireWsBase(value: unknown, apiBaseUrl: string): string {
  const normalized = normalizeWsBase(value, apiBaseUrl);
  if (!normalized) {
    throw new Error("data_analysis_backend_ws_endpoint_invalid");
  }
  return normalized;
}

export function isBrowserPreviewDataAnalysisEnvironment(): boolean {
  return typeof document !== "undefined" && document.documentElement?.dataset?.bridge === "browser-preview";
}

export function hasDesktopDataAnalysisBridge(): boolean {
  return (
    typeof window !== "undefined" &&
    !isBrowserPreviewDataAnalysisEnvironment() &&
    Boolean(window.analytix?.dataAnalysis)
  );
}

export function buildWsBaseUrlFromApiBase(apiBaseUrl: string): string {
  const apiBase = requireHttpBase(apiBaseUrl);
  const url = new URL(`${apiBase}/`);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = "/ws/events";
  url.search = "";
  url.hash = "";
  return trimTrailingSlash(url.toString());
}

function normalizeDesktopEndpointPair(
  apiBaseUrl: unknown,
  wsBaseUrl: unknown,
): { apiBaseUrl: string; wsBaseUrl: string } | null {
  const apiBase = normalizeHttpBase(apiBaseUrl);
  if (!apiBase) {
    return null;
  }
  try {
    const api = new URL(`${apiBase}/`);
    if (
      api.protocol !== "http:" ||
      api.hostname !== "127.0.0.1" ||
      !/^\d{1,5}$/u.test(api.port) ||
      Number(api.port) <= 0 ||
      Number(api.port) > 65535
    ) {
      return null;
    }
    const wsBase = normalizeWsBase(wsBaseUrl, apiBase);
    if (!wsBase) {
      return null;
    }
    return { apiBaseUrl: apiBase, wsBaseUrl: wsBase };
  } catch {
    return null;
  }
}

export function activateRuntimeBaseUrls(
  apiBaseUrl: string,
  wsBaseUrl: string,
  generation: number,
  authorityId?: string,
): { apiBaseUrl: string; wsBaseUrl: string; generation: number } {
  const endpoints = normalizeDesktopEndpointPair(apiBaseUrl, wsBaseUrl);
  const normalizedGeneration = Number(generation);
  const normalizedAuthorityId = text(authorityId) || `generation:${normalizedGeneration}`;
  if (
    !endpoints ||
    !Number.isSafeInteger(normalizedGeneration) ||
    normalizedGeneration <= 0 ||
    !normalizedAuthorityId
  ) {
    clearRuntimeBaseUrls();
    return { apiBaseUrl: "", wsBaseUrl: "", generation: 0 };
  }
  if (
    activeRuntimeGeneration !== normalizedGeneration ||
    activeRuntimeAuthorityId !== normalizedAuthorityId ||
    runtimeApiBaseOverride !== endpoints.apiBaseUrl ||
    runtimeWsBaseOverride !== endpoints.wsBaseUrl
  ) {
    runtimeAbortController?.abort();
    runtimeAbortController = new AbortController();
  }
  activeRuntimeGeneration = normalizedGeneration;
  activeRuntimeAuthorityId = normalizedAuthorityId;
  runtimeApiBaseOverride = endpoints.apiBaseUrl;
  runtimeWsBaseOverride = endpoints.wsBaseUrl;
  return {
    apiBaseUrl: endpoints.apiBaseUrl,
    wsBaseUrl: endpoints.wsBaseUrl,
    generation: normalizedGeneration,
  };
}

export function clearRuntimeBaseUrls(): void {
  runtimeAbortController?.abort();
  runtimeAbortController = null;
  activeRuntimeGeneration = 0;
  activeRuntimeAuthorityId = "";
  runtimeApiBaseOverride = "";
  runtimeWsBaseOverride = "";
}

export function getDataAnalysisRuntimeAbortSignal(): AbortSignal | undefined {
  return runtimeAbortController?.signal;
}

export function getActiveDataAnalysisRuntimeBaseLease(): DataAnalysisRuntimeBaseLease | null {
  const signal = runtimeAbortController?.signal;
  if (
    !signal ||
    signal.aborted ||
    !runtimeApiBaseOverride ||
    !runtimeWsBaseOverride ||
    activeRuntimeGeneration <= 0 ||
    !activeRuntimeAuthorityId
  ) {
    return null;
  }
  return {
    apiBaseUrl: runtimeApiBaseOverride,
    wsBaseUrl: runtimeWsBaseOverride,
    generation: activeRuntimeGeneration,
    authorityId: activeRuntimeAuthorityId,
    signal,
  };
}

export function isDataAnalysisRuntimeBaseLeaseCurrent(lease: DataAnalysisRuntimeBaseLease): boolean {
  const current = getActiveDataAnalysisRuntimeBaseLease();
  return Boolean(
    current &&
      current.signal === lease.signal &&
      current.generation === lease.generation &&
      current.authorityId === lease.authorityId &&
      current.apiBaseUrl === lease.apiBaseUrl &&
      current.wsBaseUrl === lease.wsBaseUrl,
  );
}

export function resolveDataAnalysisRuntimeEndpointAuthority(
  options: ResolveDataAnalysisRuntimeEndpointAuthorityOptions = {},
): DataAnalysisRuntimeEndpointAuthority {
  const requestedApiBase = text(options.requestedApiBaseUrl);
  const requestedWsBase = text(options.requestedWsBaseUrl);
  if (isBrowserPreviewDataAnalysisEnvironment()) {
    throw new Error("data_analysis_browser_preview_unavailable");
  }
  if (!hasDesktopDataAnalysisBridge()) {
    throw new Error("data_analysis_backend_authority_missing");
  }
  const lease = getActiveDataAnalysisRuntimeBaseLease();
  if (!lease) {
    throw new Error("data_analysis_backend_authority_missing");
  }
  if (requestedApiBase && requireHttpBase(requestedApiBase) !== lease.apiBaseUrl) {
    throw new Error("data_analysis_backend_endpoint_stale");
  }
  if (requestedWsBase && requireWsBase(requestedWsBase, lease.apiBaseUrl) !== lease.wsBaseUrl) {
    throw new Error("data_analysis_backend_endpoint_stale");
  }
  return {
    kind: "desktop_lease",
    apiBaseUrl: lease.apiBaseUrl,
    wsBaseUrl: lease.wsBaseUrl,
    authorityId: lease.authorityId,
    lease,
  };
}

export function isDataAnalysisRuntimeEndpointAuthorityCurrent(
  authority: DataAnalysisRuntimeEndpointAuthority,
): boolean {
  return hasDesktopDataAnalysisBridge() && isDataAnalysisRuntimeBaseLeaseCurrent(authority.lease);
}
