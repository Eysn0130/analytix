import { httpClient } from "../http/client";
import {
  readFlowDirectBuildFacts,
} from "./flow-direct-build-policy";

export type AnalysisFlowDirection = "in" | "out" | "both";

export interface AnalysisFlowBuildJobReq extends Record<string, unknown> {
  case_id: string;
  seeds: string[];
  depth: number;
  direction: AnalysisFlowDirection;
  min_amount: number;
  source?: string;
  request_id?: string;
  view?: string;
  layout?: string;
  date_start?: string;
  date_end?: string;
  focus_id?: string;
  focus_name?: string;
  focus_key_type?: string;
  focus_label?: string;
  focus_only?: boolean;
  focus_unknown_name?: boolean;
  include_missing_counterparty?: boolean;
  focus_counterparty_strict?: boolean;
  left_seeds?: string[];
  expected_total_amount?: number | null;
  expected_row_count?: number;
  focus_ids?: string[];
  focus_names?: string[];
  focus_placeholder_kinds?: string[];
}

export interface AnalysisFlowBuildJobDTO {
  job_id: string;
  case_id: string;
  status: "queued" | "running" | "succeeded" | "failed" | "canceled";
  progress: number;
  summary: Record<string, unknown>;
  error: string | null;
  result_available: boolean;
  created_at: string;
  updated_at: string;
}

const FLOW_DIRECT_WARM_TRACE_PREFIX = "flow-warm";

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

function shouldUseDirectBuildPath(payload: Record<string, unknown>): boolean {
  return readFlowDirectBuildFacts(payload) !== null;
}

function createTraceId(prefix: string): string {
  const normalizedPrefix = text(prefix) || "flow";
  const nonce = Math.random().toString(36).slice(2, 8);
  return `${normalizedPrefix}:${Date.now().toString(36)}:${nonce}`;
}

class StatsFlowWarmService {
  private caseId = "";
  private readonly activeRequests = new Set<AbortController>();

  setCaseId(caseId: string): void {
    const nextCaseId = text(caseId);
    if (nextCaseId === this.caseId) {
      return;
    }
    this.caseId = nextCaseId;
    for (const controller of this.activeRequests) {
      controller.abort();
    }
    this.activeRequests.clear();
  }

  async warmFlowGraph(payload: Record<string, unknown>): Promise<void> {
    const requestPayload = { ...(payload as Record<string, unknown>) };
    const caseId = text(requestPayload.case_id || requestPayload.caseId || this.caseId);
    if (!caseId || (this.caseId && this.caseId !== caseId) || !shouldUseDirectBuildPath(requestPayload)) {
      return;
    }
    requestPayload.case_id = caseId;
    delete requestPayload.traceId;
    requestPayload.trace_id = createTraceId(FLOW_DIRECT_WARM_TRACE_PREFIX);
    const controller = new AbortController();
    this.activeRequests.add(controller);
    try {
      await httpClient.post("/api/v1/analysis/flow/graph", requestPayload, {
        signal: controller.signal,
      });
      if (controller.signal.aborted || this.caseId !== caseId) {
        throw new Error("stats_flow_warm_context_revoked");
      }
    } finally {
      this.activeRequests.delete(controller);
    }
  }
}

const sharedStatsFlowWarmService = new StatsFlowWarmService();

export function getSharedStatsFlowWarmService(): StatsFlowWarmService {
  return sharedStatsFlowWarmService;
}

export async function createAnalysisFlowBuildJob(payload: AnalysisFlowBuildJobReq): Promise<AnalysisFlowBuildJobDTO> {
  const response = await httpClient.post<AnalysisFlowBuildJobDTO>("/api/v1/analysis/flow/jobs", payload);
  return response.data;
}

export async function cancelAnalysisFlowBuildJob(jobId: string): Promise<void> {
  await httpClient.post<{ ok: boolean }>(`/api/v1/analysis/flow/jobs/${encodeURIComponent(text(jobId))}/cancel`);
}
