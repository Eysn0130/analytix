import {
  isDataAnalysisRuntimeEndpointAuthorityCurrent,
  resolveDataAnalysisRuntimeEndpointAuthority,
} from "../runtime-base";
import { ensureDesktopBackendRuntime, hasDesktopBridge } from "../desktop/client";

export interface BackendHealth {
  status: "ok" | "degraded" | "alive" | string;
  service: string;
  version: string;
  env: string;
  phase: string;
  uptime_s: number;
  cleaning_native_available?: boolean;
  cleaning_native_reason?: string;
  cleaning_native_bin?: string;
  cleaning_native_bin_source?: string;
  legacy_python_cleaning_allowed?: boolean;
  checks: Record<string, boolean>;
}

interface EnvelopeHealthPayload {
  data?: BackendHealth;
}

function isBackendHealth(value: unknown): value is BackendHealth {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as Partial<BackendHealth>;
  return typeof candidate.status === "string" && typeof candidate.service === "string";
}

export async function fetchBackendHealth(): Promise<BackendHealth> {
  if (hasDesktopBridge()) {
    const ready = await ensureDesktopBackendRuntime();
    if (!ready) {
      throw new Error("data_analysis_backend_not_ready");
    }
  }
  const authority = resolveDataAnalysisRuntimeEndpointAuthority();
  const response = await fetch(`${authority.apiBaseUrl}/health/ready`, {
    method: "GET",
    signal: authority.lease?.signal,
    cache: "no-store",
    redirect: "error",
  });
  const payload = (await response.json()) as BackendHealth | EnvelopeHealthPayload;

  if (!isDataAnalysisRuntimeEndpointAuthorityCurrent(authority)) {
    throw new Error("data_analysis_backend_generation_stale");
  }

  if (!response.ok) {
    throw new Error(`backend_health_http_${response.status}`);
  }

  if (isBackendHealth(payload)) {
    return payload;
  }

  if (isBackendHealth(payload?.data)) {
    return payload.data;
  }

  throw new Error("invalid_backend_health_payload");
}
