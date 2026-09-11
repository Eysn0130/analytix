import {
  getActiveDataAnalysisRuntimeBaseLease,
  isDataAnalysisRuntimeEndpointAuthorityCurrent,
  resolveDataAnalysisRuntimeEndpointAuthority,
  type DataAnalysisRuntimeEndpointAuthority,
  type DataAnalysisRuntimeBaseLease,
} from "../runtime-base";
import { ensureDesktopBackendRuntime, hasDesktopBridge } from "../desktop/client";

export interface ApiMeta {
  request_id: string;
  timestamp: string;
}

export interface ApiSuccess<T> extends ApiMeta {
  data: T;
}

export interface HttpClientTiming {
  fetchMs: number;
  textMs: number;
  jsonParseMs: number;
  totalMs: number;
  responseBytes: number;
  contentLength: number;
  jsonBytesHeader: number;
  status: number;
  contentEncoding: string;
  requestDurationMs: number;
  serverTiming: string;
  serverTimingMetrics: Record<string, number>;
  serverTimingTotalMs: number;
  resourceTimingAvailable: boolean;
  resourceDurationMs: number;
  resourceStartDelayMs: number;
  resourceQueueMs: number;
  resourceDnsMs: number;
  resourceConnectMs: number;
  resourceSecureConnectionMs: number;
  resourceRequestMs: number;
  resourceFetchToResponseMs: number;
  resourceTtfbMs: number;
  resourceResponseBodyMs: number;
  resourceTransferSize: number;
  resourceEncodedBodySize: number;
  resourceDecodedBodySize: number;
  resourceNextHopProtocol: string;
}

export interface ApiTimedSuccess<T> {
  response: ApiSuccess<T>;
  timing: HttpClientTiming;
}

export interface ApiErrorPayload extends ApiMeta {
  error: {
    code: string;
    message: string;
    retryable: boolean;
    details?: Record<string, unknown>;
  };
}

export class ApiClientError extends Error {
  readonly status: number;
  readonly code: string;
  readonly retryable: boolean;
  readonly details: Record<string, unknown>;

  constructor(status: number, payload: unknown) {
    const normalized = normalizeApiErrorPayload(status, payload);
    super(normalized.error.message);
    this.name = "ApiClientError";
    this.status = status;
    this.code = normalized.error.code;
    this.retryable = Boolean(normalized.error.retryable);
    this.details = normalized.error.details ?? {};
  }
}

function normalizeApiErrorPayload(status: number, payload: unknown): ApiErrorPayload {
  if (
    payload &&
    typeof payload === "object" &&
    "error" in payload &&
    payload.error &&
    typeof payload.error === "object" &&
    "message" in payload.error &&
    typeof payload.error.message === "string"
  ) {
    return payload as ApiErrorPayload;
  }
  const fallbackMessage =
    payload && typeof payload === "object" && "detail" in payload && typeof payload.detail === "string"
      ? payload.detail
      : `HTTP ${status}`;
  return {
    request_id: "",
    timestamp: "",
    error: {
      code: "HTTP_ERROR",
      message: fallbackMessage,
      retryable: status >= 500,
      details: payload && typeof payload === "object" ? (payload as Record<string, unknown>) : {}
    }
  };
}

function trimTrailingSlash(value: string): string {
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

async function ensureHttpRuntimeBase(): Promise<DataAnalysisRuntimeBaseLease | null> {
  if (!hasDesktopBridge()) {
    return null;
  }
  const ready = await ensureDesktopBackendRuntime();
  if (!ready) {
    throw new Error("数据分析后端服务未就绪，请稍后重试。");
  }
  const lease = getActiveDataAnalysisRuntimeBaseLease();
  if (!lease) {
    throw new Error("data_analysis_backend_authority_missing");
  }
  return lease;
}

function resolveRequestAbortSignal(
  callerSignal?: AbortSignal | null,
  capturedRuntimeSignal?: AbortSignal,
): AbortSignal | undefined {
  const runtimeSignal = capturedRuntimeSignal;
  if (!callerSignal) {
    return runtimeSignal;
  }
  if (!runtimeSignal || callerSignal === runtimeSignal) {
    return callerSignal;
  }
  return AbortSignal.any([callerSignal, runtimeSignal]);
}

function readHttpNow(): number {
  const perf = globalThis.performance;
  if (perf && typeof perf.now === "function") {
    return perf.now();
  }
  return Date.now();
}

function roundHttpMs(value: number): number {
  return Math.round(Math.max(0, Number(value) || 0) * 1000) / 1000;
}

function utf8ByteLength(value: string): number {
  if (typeof TextEncoder !== "undefined") {
    return new TextEncoder().encode(value).length;
  }
  return value.length;
}

function readHeaderInt(headers: Headers, name: string): number {
  const raw = headers.get(name);
  const value = Number(raw);
  return Number.isFinite(value) && value >= 0 ? Math.floor(value) : 0;
}

function readHeaderMs(headers: Headers, name: string): number {
  const raw = headers.get(name);
  const value = Number(raw);
  return Number.isFinite(value) && value >= 0 ? roundHttpMs(value) : 0;
}

function readHeaderText(headers: Headers, name: string): string {
  return String(headers.get(name) || "").trim();
}

function parseServerTimingHeader(value: string): Record<string, number> {
  const out: Record<string, number> = {};
  String(value || "")
    .split(",")
    .forEach((rawPart) => {
      const part = rawPart.trim();
      if (!part) {
        return;
      }
      const [rawName, ...rawParams] = part.split(";").map((item) => item.trim());
      const name = rawName.trim();
      if (!name) {
        return;
      }
      const durationParam = rawParams.find((item) => item.toLowerCase().startsWith("dur="));
      if (!durationParam) {
        return;
      }
      const duration = Number(durationParam.slice(durationParam.indexOf("=") + 1).replace(/^"|"$/g, ""));
      if (Number.isFinite(duration) && duration >= 0) {
        out[name] = duration;
      }
    });
  return out;
}

function resolveServerTimingTotalMs(metrics: Record<string, number>): number {
  const totals = Object.entries(metrics)
    .filter(([name, value]) => (name === "total" || name.endsWith(".total")) && Number.isFinite(value) && value >= 0)
    .map(([, value]) => value);
  return totals.length ? roundHttpMs(Math.max(...totals)) : 0;
}

interface HttpResourceTimingSnapshot {
  available: boolean;
  durationMs: number;
  startDelayMs: number;
  queueMs: number;
  dnsMs: number;
  connectMs: number;
  secureConnectionMs: number;
  requestMs: number;
  fetchToResponseMs: number;
  ttfbMs: number;
  responseBodyMs: number;
  transferSize: number;
  encodedBodySize: number;
  decodedBodySize: number;
  nextHopProtocol: string;
}

const EMPTY_HTTP_RESOURCE_TIMING: HttpResourceTimingSnapshot = {
  available: false,
  durationMs: 0,
  startDelayMs: 0,
  queueMs: 0,
  dnsMs: 0,
  connectMs: 0,
  secureConnectionMs: 0,
  requestMs: 0,
  fetchToResponseMs: 0,
  ttfbMs: 0,
  responseBodyMs: 0,
  transferSize: 0,
  encodedBodySize: 0,
  decodedBodySize: 0,
  nextHopProtocol: "",
};

function readLatestResourceTiming(url: string, fetchStartedAt: number): HttpResourceTimingSnapshot {
  const perf = globalThis.performance;
  if (!perf || typeof perf.getEntriesByName !== "function") {
    return EMPTY_HTTP_RESOURCE_TIMING;
  }
  const entries = perf
    .getEntriesByName(url, "resource")
    .filter((entry): entry is PerformanceResourceTiming => "responseEnd" in entry)
    .filter((entry) => Number(entry.startTime) >= fetchStartedAt - 5)
    .sort((left, right) => Number(right.startTime) - Number(left.startTime));
  const entry = entries[0];
  if (!entry || Number(entry.responseStart || 0) <= 0 || Number(entry.responseEnd || 0) <= 0) {
    return EMPTY_HTTP_RESOURCE_TIMING;
  }
  const startTime = Number(entry.startTime || 0);
  const fetchStart = Number(entry.fetchStart || 0) > 0 ? Number(entry.fetchStart) : startTime;
  const domainLookupStart = Number(entry.domainLookupStart || 0);
  const domainLookupEnd = Number(entry.domainLookupEnd || 0);
  const connectStart = Number(entry.connectStart || 0);
  const connectEnd = Number(entry.connectEnd || 0);
  const secureConnectionStart = Number(entry.secureConnectionStart || 0);
  const requestStart = Number(entry.requestStart || 0) > 0 ? Number(entry.requestStart) : startTime;
  const responseStart = Number(entry.responseStart || 0);
  const responseEnd = Number(entry.responseEnd || 0);
  const requestMs = roundHttpMs(Math.max(0, responseStart - requestStart));
  return {
    available: true,
    durationMs: roundHttpMs(Number(entry.duration || responseEnd - startTime)),
    startDelayMs: roundHttpMs(Math.max(0, startTime - fetchStartedAt)),
    queueMs: roundHttpMs(Math.max(0, requestStart - startTime)),
    dnsMs: roundHttpMs(Math.max(0, domainLookupEnd - domainLookupStart)),
    connectMs: roundHttpMs(Math.max(0, connectEnd - connectStart)),
    secureConnectionMs: roundHttpMs(secureConnectionStart > 0 ? Math.max(0, connectEnd - secureConnectionStart) : 0),
    requestMs,
    fetchToResponseMs: roundHttpMs(Math.max(0, responseStart - fetchStart)),
    ttfbMs: requestMs,
    responseBodyMs: roundHttpMs(Math.max(0, responseEnd - responseStart)),
    transferSize: Math.max(0, Math.floor(Number(entry.transferSize || 0))),
    encodedBodySize: Math.max(0, Math.floor(Number(entry.encodedBodySize || 0))),
    decodedBodySize: Math.max(0, Math.floor(Number(entry.decodedBodySize || 0))),
    nextHopProtocol: String(entry.nextHopProtocol || ""),
  };
}

export class HttpClient {
  private readonly baseUrl: string | null;
  private readonly boundRuntimeAuthorityId: string | null;

  constructor(baseUrl?: string) {
    this.baseUrl = baseUrl ? trimTrailingSlash(baseUrl) : null;
    this.boundRuntimeAuthorityId = this.baseUrl && hasDesktopBridge()
      ? getActiveDataAnalysisRuntimeBaseLease()?.authorityId ?? null
      : null;
  }

  async get<T>(path: string, init: RequestInit = {}): Promise<ApiSuccess<T>> {
    return this.request<T>(path, {
      ...init,
      method: "GET"
    });
  }

  async post<T>(path: string, body?: unknown, init: RequestInit = {}): Promise<ApiSuccess<T>> {
    return this.request<T>(path, {
      ...init,
      method: "POST",
      body: body === undefined ? undefined : JSON.stringify(body),
      headers: {
        "Content-Type": "application/json",
        ...(init.headers ?? {})
      }
    });
  }

  async postTimed<T>(path: string, body?: unknown, init: RequestInit = {}): Promise<ApiTimedSuccess<T>> {
    return this.requestTimed<T>(path, {
      ...init,
      method: "POST",
      body: body === undefined ? undefined : JSON.stringify(body),
      headers: {
        "Content-Type": "application/json",
        ...(init.headers ?? {})
      }
    });
  }

  async put<T>(path: string, body?: unknown, init: RequestInit = {}): Promise<ApiSuccess<T>> {
    return this.request<T>(path, {
      ...init,
      method: "PUT",
      body: body === undefined ? undefined : JSON.stringify(body),
      headers: {
        "Content-Type": "application/json",
        ...(init.headers ?? {})
      }
    });
  }

  async patch<T>(path: string, body?: unknown, init: RequestInit = {}): Promise<ApiSuccess<T>> {
    return this.request<T>(path, {
      ...init,
      method: "PATCH",
      body: body === undefined ? undefined : JSON.stringify(body),
      headers: {
        "Content-Type": "application/json",
        ...(init.headers ?? {})
      }
    });
  }

  async delete<T>(path: string, init: RequestInit = {}): Promise<ApiSuccess<T>> {
    return this.request<T>(path, {
      ...init,
      method: "DELETE"
    });
  }

  private async request<T>(path: string, init: RequestInit): Promise<ApiSuccess<T>> {
    return (await this.requestTimed<T>(path, init)).response;
  }

  private async requestTimed<T>(path: string, init: RequestInit): Promise<ApiTimedSuccess<T>> {
    if (hasDesktopBridge()) {
      await ensureHttpRuntimeBase();
    }
    const authority = resolveDataAnalysisRuntimeEndpointAuthority({
      requestedApiBaseUrl: this.baseUrl ?? undefined,
    });
    const runtimeLease = authority.lease;
    if (
      runtimeLease &&
      this.boundRuntimeAuthorityId &&
      this.boundRuntimeAuthorityId !== runtimeLease.authorityId
    ) {
      throw new Error("data_analysis_backend_generation_stale");
    }
    const baseUrl = authority.apiBaseUrl;
    const url = `${baseUrl}${path.startsWith("/") ? path : `/${path}`}`;
    return this.fetchTimed<T>(url, init, authority);
  }

  private async fetchTimed<T>(
    url: string,
    init: RequestInit,
    authority: DataAnalysisRuntimeEndpointAuthority,
  ): Promise<ApiTimedSuccess<T>> {
    const runtimeLease = authority.lease;
    const startedAt = readHttpNow();
    const fetchStartedAt = readHttpNow();
    const response = await fetch(url, {
      ...init,
      signal: resolveRequestAbortSignal(init.signal, runtimeLease?.signal),
      cache: "no-store",
      redirect: "error",
    });
    if (!isDataAnalysisRuntimeEndpointAuthorityCurrent(authority)) {
      throw new Error("data_analysis_backend_generation_stale");
    }
    const fetchEndedAt = readHttpNow();
    const textStartedAt = readHttpNow();
    const rawText = await response.text();
    if (!isDataAnalysisRuntimeEndpointAuthorityCurrent(authority)) {
      throw new Error("data_analysis_backend_generation_stale");
    }
    const textEndedAt = readHttpNow();
    let payload: unknown = {};
    const parseStartedAt = readHttpNow();
    try {
      payload = rawText ? JSON.parse(rawText) : {};
    } catch {
      payload = rawText ? { detail: rawText } : { detail: response.statusText || `HTTP ${response.status}` };
    }
    const parseEndedAt = readHttpNow();
    const serverTiming = readHeaderText(response.headers, "Server-Timing");
    const serverTimingMetrics = parseServerTimingHeader(serverTiming);
    const resourceTiming = readLatestResourceTiming(url, fetchStartedAt);
    const timing: HttpClientTiming = {
      fetchMs: roundHttpMs(fetchEndedAt - fetchStartedAt),
      textMs: roundHttpMs(textEndedAt - textStartedAt),
      jsonParseMs: roundHttpMs(parseEndedAt - parseStartedAt),
      totalMs: roundHttpMs(parseEndedAt - startedAt),
      responseBytes: utf8ByteLength(rawText),
      contentLength: readHeaderInt(response.headers, "Content-Length"),
      jsonBytesHeader: readHeaderInt(response.headers, "X-Analytix-Json-Bytes"),
      status: response.status,
      contentEncoding: readHeaderText(response.headers, "X-Analytix-Content-Encoding") ||
        readHeaderText(response.headers, "Content-Encoding") ||
        "identity",
      requestDurationMs: readHeaderMs(response.headers, "X-Analytix-Request-Duration-Ms"),
      serverTiming,
      serverTimingMetrics,
      serverTimingTotalMs: resolveServerTimingTotalMs(serverTimingMetrics),
      resourceTimingAvailable: resourceTiming.available,
      resourceDurationMs: resourceTiming.durationMs,
      resourceStartDelayMs: resourceTiming.startDelayMs,
      resourceQueueMs: resourceTiming.queueMs,
      resourceDnsMs: resourceTiming.dnsMs,
      resourceConnectMs: resourceTiming.connectMs,
      resourceSecureConnectionMs: resourceTiming.secureConnectionMs,
      resourceRequestMs: resourceTiming.requestMs,
      resourceFetchToResponseMs: resourceTiming.fetchToResponseMs,
      resourceTtfbMs: resourceTiming.ttfbMs,
      resourceResponseBodyMs: resourceTiming.responseBodyMs,
      resourceTransferSize: resourceTiming.transferSize,
      resourceEncodedBodySize: resourceTiming.encodedBodySize,
      resourceDecodedBodySize: resourceTiming.decodedBodySize,
      resourceNextHopProtocol: resourceTiming.nextHopProtocol
    };

    if (!response.ok) {
      throw new ApiClientError(response.status, payload);
    }

    return {
      response: payload as ApiSuccess<T>,
      timing
    };
  }
}

export const httpClient = new HttpClient();
