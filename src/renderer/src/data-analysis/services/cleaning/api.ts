import { httpClient } from "../http/client";

export type JobStatus = "queued" | "running" | "succeeded" | "failed" | "canceled";

export interface PaginationMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
  has_next: boolean;
}

export interface CleaningJobReq {
  case_id: string;
  steps: number[];
  force_rebuild: boolean;
}

export interface CleaningResetReq {
  reason?: string;
}

export interface CleaningJobDTO {
  job_id: string;
  case_id: string;
  status: JobStatus;
  progress: number;
  result_semantic_status: "blocked";
  result_blocker: "host_evidence_receipt_required";
  fact_answer_allowed: false;
  cleaned_rows: number | null;
  summary: Record<string, unknown>;
  error: string | null;
  created_at: string;
  updated_at: string;
}

export interface CleaningJobListData {
  items: CleaningJobDTO[];
  page: PaginationMeta;
}

export interface CleaningStepSummaryDTO {
  step: number;
  key: string;
  title: string;
  kind: string;
  description: string;
  affected_rows: null;
}

export interface CleaningStepSummaryListData {
  contract: "CleaningStepSummaryPublicBoundaryV1";
  case_id: string;
  semantic_status: "blocked";
  blocker: "host_evidence_receipt_required";
  publication_status: "blocked";
  fact_answer_allowed: false;
  items: CleaningStepSummaryDTO[];
}

export interface CleaningStepDetailRowDTO {
  values: string[];
}

export interface CleaningStepDetailDTO {
  contract: "CleaningStepDetailPublicBoundaryV1";
  case_id: string;
  semantic_status: "blocked";
  blocker: "host_evidence_receipt_required";
  publication_status: "blocked";
  fact_answer_allowed: false;
  raw_details_exposed: false;
  step: number;
  key: string;
  title: string;
  kind: string;
  description: string;
  headers: string[];
  items: CleaningStepDetailRowDTO[];
  page: null;
}

export interface CleaningHistoryItemDTO {
  run_id: string;
  cleaned_at: string;
  import_at: string;
  duration_ms: number | null;
  scope_rows: number | null;
  file_count: number | null;
  summary: Record<string, unknown>;
}

export interface CleaningHistoryListData {
  contract: "CleaningHistoryPublicBoundaryV1";
  case_id: string;
  semantic_status: "blocked";
  blocker: "host_evidence_receipt_required";
  fact_answer_allowed: false;
  items: CleaningHistoryItemDTO[];
}

export interface CleaningLogEventDTO {
  event_id: string;
  job_id: string;
  event: string;
  level: string;
  message_code: string;
  message: string;
  step: number | null;
  progress: number | null;
  timestamp: string;
}

export interface CleaningLogListData {
  contract: "CleaningLogPublicBoundaryV1";
  case_id: string;
  semantic_status: "blocked";
  blocker: "host_evidence_receipt_required";
  fact_answer_allowed: false;
  items: CleaningLogEventDTO[];
}

function buildJobsQuery(params: {
  caseId: string;
  page?: number;
  pageSize?: number;
  status?: JobStatus | "";
}): string {
  const query = new URLSearchParams();
  query.set("case_id", params.caseId);
  query.set("page", String(params.page ?? 1));
  query.set("page_size", String(params.pageSize ?? 50));
  if ((params.status ?? "").trim()) {
    query.set("status", params.status ?? "");
  }
  return query.toString();
}

function cleaningCanonicalProgress(value: unknown): number | null {
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    value > 100 ||
    Object.is(value, -0)
  ) {
    return null;
  }
  return value;
}

function cleaningCanonicalStep(value: unknown): number | null {
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 1 ||
    value > 10 ||
    Object.is(value, -0)
  ) {
    return null;
  }
  return value;
}

export async function createCleaningJob(payload: CleaningJobReq): Promise<CleaningJobDTO> {
  const response = await httpClient.post<CleaningJobDTO>("/api/v1/cleaning/jobs", payload);
  return normalizeCleaningJobBoundary(response.data, payload.case_id);
}

export async function listCleaningJobs(params: {
  caseId: string;
  page?: number;
  pageSize?: number;
  status?: JobStatus | "";
}): Promise<CleaningJobListData> {
  const query = buildJobsQuery(params);
  const response = await httpClient.get<CleaningJobListData>(`/api/v1/cleaning/jobs?${query}`);
  return {
    ...response.data,
    items: Array.isArray(response.data.items)
      ? response.data.items.map((item) => normalizeCleaningJobBoundary(item, params.caseId))
      : []
  };
}

export async function getCleaningJob(jobId: string, caseId: string): Promise<CleaningJobDTO> {
  const query = new URLSearchParams({ case_id: caseId });
  const response = await httpClient.get<CleaningJobDTO>(
    `/api/v1/cleaning/jobs/${encodeURIComponent(jobId)}?${query.toString()}`
  );
  return normalizeCleaningJobBoundary(response.data, caseId);
}

export async function resetCleaningJob(jobId: string, caseId: string, payload?: CleaningResetReq): Promise<CleaningJobDTO> {
  const query = new URLSearchParams({ case_id: caseId });
  const response = await httpClient.post<CleaningJobDTO>(
    `/api/v1/cleaning/jobs/${encodeURIComponent(jobId)}/reset?${query.toString()}`,
    payload
  );
  return normalizeCleaningJobBoundary(response.data, caseId);
}

export async function cancelCleaningJob(jobId: string, caseId: string): Promise<void> {
  const query = new URLSearchParams({ case_id: caseId });
  await httpClient.post<{ ok: boolean }>(
    `/api/v1/cleaning/jobs/${encodeURIComponent(jobId)}/cancel?${query.toString()}`
  );
}

export async function listCleaningStepSummaries(caseId: string): Promise<CleaningStepSummaryListData> {
  const query = new URLSearchParams();
  query.set("case_id", caseId);
  const response = await httpClient.get<CleaningStepSummaryListData>(`/api/v1/cleaning/steps?${query.toString()}`);
  return normalizeCleaningStepSummaryBoundary(response.data, caseId);
}

export async function getCleaningStepDetail(params: {
  caseId: string;
  step: number;
  page?: number;
  pageSize?: number;
  offset?: number;
}): Promise<CleaningStepDetailDTO> {
  const query = new URLSearchParams();
  query.set("case_id", params.caseId);
  query.set("page", String(params.page ?? 1));
  query.set("page_size", String(params.pageSize ?? 20));
  if (typeof params.offset === "number" && Number.isFinite(params.offset) && params.offset >= 0) {
    query.set("offset", String(Math.floor(params.offset)));
  }
  const response = await httpClient.get<CleaningStepDetailDTO>(
    `/api/v1/cleaning/steps/${encodeURIComponent(String(params.step))}?${query.toString()}`
  );
  return normalizeCleaningStepDetailBoundary(response.data, params.caseId, params.step);
}

export function normalizeCleaningStepSummaryBoundary(
  raw: unknown,
  expectedCaseId: string
): CleaningStepSummaryListData {
  const source = raw && typeof raw === "object" ? raw as Record<string, unknown> : {};
  if (typeof source.case_id !== "string" || source.case_id !== expectedCaseId) {
    throw new Error("cleaning_case_binding_mismatch");
  }
  const rawItems = Array.isArray(source.items) ? source.items : [];
  const items = rawItems.flatMap((item): CleaningStepSummaryDTO[] => {
    if (!item || typeof item !== "object") {
      return [];
    }
    const candidate = item as Record<string, unknown>;
    const step = cleaningCanonicalStep(candidate.step);
    if (step === null) {
      return [];
    }
    return [{
      step,
      key: String(candidate.key || ""),
      title: String(candidate.title || ""),
      kind: String(candidate.kind || ""),
      description: String(candidate.description || ""),
      affected_rows: null
    }];
  });
  return {
    contract: "CleaningStepSummaryPublicBoundaryV1",
    case_id: expectedCaseId,
    semantic_status: "blocked",
    blocker: "host_evidence_receipt_required",
    publication_status: "blocked",
    fact_answer_allowed: false,
    items
  };
}

export function normalizeCleaningJobBoundary(raw: unknown, expectedCaseId: string): CleaningJobDTO {
  const source = raw && typeof raw === "object" ? raw as Record<string, unknown> : {};
  if (typeof source.case_id !== "string" || source.case_id !== expectedCaseId) {
    throw new Error("cleaning_case_binding_mismatch");
  }
  const status = String(source.status || "");
  if (!["queued", "running", "succeeded", "failed", "canceled"].includes(status)) {
    throw new Error("cleaning_job_status_invalid");
  }
  const progress = cleaningCanonicalProgress(source.progress);
  if (progress === null) {
    throw new Error("cleaning_job_progress_invalid");
  }
  return {
    job_id: String(source.job_id || ""),
    case_id: expectedCaseId,
    status: status as JobStatus,
    progress,
    result_semantic_status: "blocked",
    result_blocker: "host_evidence_receipt_required",
    fact_answer_allowed: false,
    cleaned_rows: null,
    summary: {},
    error: typeof source.error === "string" ? source.error : null,
    created_at: String(source.created_at || ""),
    updated_at: String(source.updated_at || "")
  };
}

export function normalizeCleaningStepDetailBoundary(
  raw: unknown,
  expectedCaseId: string,
  expectedStep: number
): CleaningStepDetailDTO {
  const source = raw && typeof raw === "object" ? raw as Record<string, unknown> : {};
  if (
    typeof source.case_id !== "string" ||
    source.case_id !== expectedCaseId ||
    cleaningCanonicalStep(source.step) !== expectedStep
  ) {
    throw new Error("cleaning_case_binding_mismatch");
  }
  return {
    contract: "CleaningStepDetailPublicBoundaryV1",
    case_id: expectedCaseId,
    semantic_status: "blocked",
    blocker: "host_evidence_receipt_required",
    publication_status: "blocked",
    fact_answer_allowed: false,
    raw_details_exposed: false,
    step: expectedStep,
    key: String(source.key || ""),
    title: String(source.title || ""),
    kind: String(source.kind || ""),
    description: String(source.description || ""),
    headers: Array.isArray(source.headers) ? source.headers.map((item) => String(item || "")) : [],
    items: [],
    page: null
  };
}

export async function listCleaningHistory(caseId: string, limit = 50): Promise<CleaningHistoryListData> {
  const query = new URLSearchParams();
  query.set("case_id", caseId);
  query.set("limit", String(limit));
  const response = await httpClient.get<CleaningHistoryListData>(`/api/v1/cleaning/history?${query.toString()}`);
  return response.data;
}

export async function listCleaningLogs(caseId: string, limit = 200): Promise<CleaningLogListData> {
  const query = new URLSearchParams();
  query.set("case_id", caseId);
  query.set("limit", String(limit));
  const response = await httpClient.get<CleaningLogListData>(`/api/v1/cleaning/logs?${query.toString()}`);
  return response.data;
}
