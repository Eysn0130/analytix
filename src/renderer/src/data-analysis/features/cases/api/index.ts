import { httpClient } from "../../../services/http/client";
import {
  requireControlledArtifactPublication,
  requireControlledSourceIngestion,
} from "../../../services/publication-quarantine";
import {
  normalizeCaseTags,
  normalizeCaseType,
} from "../model/case-taxonomy";

export interface PaginationMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
  has_next: boolean;
}

export type ImportHealth = "ok" | "warn" | "fail" | "unknown";
export type CaseStatus = "active" | "archived";

export interface CaseListItemDTO {
  case_id: string;
  case_name: string;
  case_number: string;
  owner: string;
  note: string;
  case_type: string;
  tags: string[];
  status: CaseStatus;
  is_deleted: boolean;
  size_label: string;
  size_bytes: number | null;
  size_status: "available" | "unavailable";
  import_health: ImportHealth;
  import_source_status: "available" | "unavailable";
  created_at: string;
  updated_at: string;
}

export interface CaseStatsDTO {
  tasks: number | null;
  accounts: number | null;
  persons: number | null;
  tx: number | null;
  sub: number | null;
}

export interface CaseImportLogDTO {
  title: string;
  time: string;
  status: ImportHealth;
  msg: string;
  rows_total: number | null;
  rows_imported: number | null;
  rows_dedup: number | null;
  rows_error: number | null;
  rows_skipped_non_data: number | null;
  error: string;
}

export interface CaseDetailDTO extends CaseListItemDTO {
  stats: CaseStatsDTO;
  stats_source_status: "available" | "unavailable";
  imports: CaseImportLogDTO[];
  imports_source_status: "available" | "unavailable";
}

const IMPORT_HEALTH_VALUES = new Set<ImportHealth>(["ok", "warn", "fail", "unknown"]);

function knownCaseImportCount(value: unknown): number | null {
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    Object.is(value, -0)
  ) {
    return null;
  }
  return value;
}

export function normalizeCaseImportLogDTO(value: unknown): CaseImportLogDTO {
  const record = value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
  const rowsTotal = knownCaseImportCount(record.rows_total);
  const rowsImported = knownCaseImportCount(record.rows_imported);
  const rowsDedup = knownCaseImportCount(record.rows_dedup);
  const rowsError = knownCaseImportCount(record.rows_error);
  const rowsSkippedNonData = knownCaseImportCount(record.rows_skipped_non_data);
  const rawStatus = typeof record.status === "string" ? record.status : "unknown";
  const projectedStatus = IMPORT_HEALTH_VALUES.has(rawStatus as ImportHealth)
    ? rawStatus as ImportHealth
    : "unknown";
  const status = rowsImported === null && projectedStatus === "ok" ? "unknown" : projectedStatus;
  let msg = rowsImported === null ? "导入数量未知" : `导入 ${rowsImported} 条`;
  if (rowsDedup !== null && rowsDedup > 0) {
    msg += ` · 去重 ${rowsDedup} 条`;
  }
  if (rowsError !== null && rowsError > 0) {
    msg += ` · 错误 ${rowsError} 条`;
  }
  return {
    title: typeof record.title === "string" ? record.title : "Import",
    time: typeof record.time === "string" ? record.time : "",
    status,
    msg,
    rows_total: rowsTotal,
    rows_imported: rowsImported,
    rows_dedup: rowsDedup,
    rows_error: rowsError,
    rows_skipped_non_data: rowsSkippedNonData,
    error: typeof record.error === "string" ? record.error : "",
  };
}

export interface CaseAuditItemDTO {
  event_version: 'case_audit_public_v1';
  time: string;
  actor: 'local_operator' | 'system';
  action:
    | 'analysis.bootstrap'
    | 'backup_case'
    | 'create_case'
    | 'delete_case'
    | 'open_case'
    | 'purge_case'
    | 'purge_import_files'
    | 'recycle_import_files'
    | 'restore_case'
    | 'restore_import_files'
    | 'stats.skill_query'
    | 'sync_case_project_doc_metadata'
    | 'unclassified_event'
    | 'update_case';
  case_id: string;
  status: 'recorded';
  details: {
    affected_count: number | null;
    changed_fields: Array<
      'case_no' | 'case_type' | 'name' | 'org' | 'owner' | 'status' | 'summary' | 'tags'
    >;
    restricted_details_withheld: boolean;
  };
}

export interface CaseListData {
  items: CaseListItemDTO[];
  page: PaginationMeta;
}

export interface CaseCreateReq {
  case_name: string;
  case_number: string;
  owner?: string;
  note?: string;
  case_type?: string;
  tags?: string[];
}

export interface CaseUpdateReq {
  case_name?: string;
  case_number?: string;
  owner?: string;
  note?: string;
  case_type?: string;
  tags?: string[];
  status?: CaseStatus;
}

export interface CaseArchiveExportReq {
  target_dir?: string;
}

export interface CaseArchiveExportDTO {
  case_id: string;
  case_name: string;
  archive_path: string;
}

export interface CaseArchiveImportReq {
  archive_path: string;
}

export interface ExportReq {
  case_id: string;
  export_format?: "csv" | "xlsx";
  filters?: Record<string, unknown>;
  output_name?: string;
  target_dir?: string;
}

export interface ExportJobDTO {
  job_id: string;
  case_id: string;
  status: string;
  progress: number;
  output_path: null;
  error: 'task_failed' | 'task_canceled' | 'task_interrupted' | 'task_runtime_payload_unavailable' | null;
  artifact_access: 'controlled_artifact_required';
  publication_status: 'blocked';
  created_at: string;
  updated_at: string;
}

function normalizeCaseItem<T extends CaseListItemDTO>(item: T): T {
  const sizeBytes = knownCaseImportCount(item.size_bytes);
  const sizeStatus = item.size_status === "available" && sizeBytes !== null
    ? "available"
    : "unavailable";
  return {
    ...item,
    case_type: normalizeCaseType(item.case_type),
    tags: normalizeCaseTags(item.tags),
    size_bytes: sizeStatus === "available" ? sizeBytes : null,
    size_label: sizeStatus === "available" && typeof item.size_label === "string"
      ? item.size_label
      : "unknown",
    size_status: sizeStatus,
    import_source_status: item.import_source_status === "available"
      ? "available"
      : "unavailable",
  } as T;
}

function normalizeCasePayload(
  payload: CaseCreateReq | CaseUpdateReq,
): CaseCreateReq | CaseUpdateReq {
  const normalized: CaseCreateReq | CaseUpdateReq = { ...payload };
  if ("case_type" in normalized && typeof normalized.case_type === "string") {
    normalized.case_type = normalizeCaseType(normalized.case_type);
  }
  if ("tags" in normalized && normalized.tags) {
    normalized.tags = normalizeCaseTags(normalized.tags);
  }
  return normalized;
}

function buildCasesQuery(params: {
  page?: number;
  pageSize?: number;
  keyword?: string;
  includeDeleted?: boolean;
}): string {
  const query = new URLSearchParams();
  query.set("page", String(params.page ?? 1));
  query.set("page_size", String(params.pageSize ?? 50));
  if ((params.keyword ?? "").trim()) {
    query.set("keyword", (params.keyword ?? "").trim());
  }
  query.set("include_deleted", params.includeDeleted ? "true" : "false");
  return query.toString();
}

export async function listCases(params: {
  page?: number;
  pageSize?: number;
  keyword?: string;
  includeDeleted?: boolean;
} = {}): Promise<CaseListData> {
  const query = buildCasesQuery(params);
  const response = await httpClient.get<CaseListData>(`/api/v1/cases?${query}`);
  return {
    ...response.data,
    items: response.data.items.map((item) => normalizeCaseItem(item)),
  };
}

export async function getActiveCase(caseId: string): Promise<CaseListItemDTO> {
  const explicitCaseId = String(caseId ?? "").trim();
  if (!explicitCaseId) {
    throw new Error("case_id_required");
  }
  const query = new URLSearchParams({ case_id: explicitCaseId });
  const response = await httpClient.get<CaseListItemDTO>(
    `/api/v1/cases/active?${query.toString()}`,
  );
  return normalizeCaseItem(response.data);
}

export async function getCaseDetail(caseId: string): Promise<CaseDetailDTO> {
  const response = await httpClient.get<CaseDetailDTO>(`/api/v1/cases/${encodeURIComponent(caseId)}`);
  const normalized = normalizeCaseItem(response.data);
  const importsSourceStatus = response.data.imports_source_status === "available"
    ? "available"
    : "unavailable";
  return {
    ...normalized,
    stats_source_status: response.data.stats_source_status === "available"
      ? "available"
      : "unavailable",
    imports_source_status: importsSourceStatus,
    imports: importsSourceStatus === "available" && Array.isArray(response.data.imports)
      ? response.data.imports.map(normalizeCaseImportLogDTO)
      : [],
  };
}

export async function listCaseAudit(caseId: string, limit = 50): Promise<{ items: CaseAuditItemDTO[] }> {
  const query = new URLSearchParams();
  query.set("limit", String(limit));
  const response = await httpClient.get<{ items: CaseAuditItemDTO[] }>(
    `/api/v1/cases/${encodeURIComponent(caseId)}/audit?${query.toString()}`
  );
  return response.data;
}

export async function createCase(payload: CaseCreateReq): Promise<CaseListItemDTO> {
  const response = await httpClient.post<CaseListItemDTO>(
    "/api/v1/cases",
    normalizeCasePayload(payload),
  );
  return normalizeCaseItem(response.data);
}

export async function updateCase(caseId: string, payload: CaseUpdateReq): Promise<CaseListItemDTO> {
  const response = await httpClient.patch<CaseListItemDTO>(
    `/api/v1/cases/${encodeURIComponent(caseId)}`,
    normalizeCasePayload(payload),
  );
  return normalizeCaseItem(response.data);
}

export async function deleteCase(caseId: string): Promise<void> {
  await httpClient.delete<{ ok: boolean }>(`/api/v1/cases/${encodeURIComponent(caseId)}`);
}

export async function purgeCase(caseId: string): Promise<void> {
  await httpClient.delete<{ ok: boolean }>(`/api/v1/cases/${encodeURIComponent(caseId)}/purge`);
}

export async function restoreCase(caseId: string): Promise<CaseListItemDTO> {
  const response = await httpClient.post<CaseListItemDTO>(`/api/v1/cases/${encodeURIComponent(caseId)}/restore`);
  return normalizeCaseItem(response.data);
}

export async function activateCase(caseId: string): Promise<CaseListItemDTO> {
  const response = await httpClient.post<CaseListItemDTO>(`/api/v1/cases/${encodeURIComponent(caseId)}/activate`);
  return normalizeCaseItem(response.data);
}

export async function exportCaseArchive(
  caseId: string,
  payload: CaseArchiveExportReq = {},
): Promise<CaseArchiveExportDTO> {
  requireControlledArtifactPublication();
  const response = await httpClient.post<CaseArchiveExportDTO>(
    `/api/v1/cases/${encodeURIComponent(caseId)}/archive/export`,
    payload,
  );
  return response.data;
}

export async function importCaseArchive(
  payload: CaseArchiveImportReq,
): Promise<CaseListItemDTO> {
  requireControlledSourceIngestion();
  const response = await httpClient.post<CaseListItemDTO>(
    "/api/v1/cases/archive/import",
    payload,
  );
  return normalizeCaseItem(response.data);
}

export async function createRawExportJob(payload: ExportReq): Promise<ExportJobDTO> {
  requireControlledArtifactPublication();
  const response = await httpClient.post<ExportJobDTO>("/api/v1/export/raw", payload);
  return response.data;
}

export async function createCleanedExportJob(payload: ExportReq): Promise<ExportJobDTO> {
  requireControlledArtifactPublication();
  const response = await httpClient.post<ExportJobDTO>("/api/v1/export/cleaned", payload);
  return response.data;
}

export async function getExportJob(jobId: string, caseId: string): Promise<ExportJobDTO> {
  const query = new URLSearchParams({ case_id: caseId });
  const response = await httpClient.get<ExportJobDTO>(
    `/api/v1/export/jobs/${encodeURIComponent(jobId)}?${query.toString()}`,
  );
  return response.data;
}

export async function cancelExportJob(jobId: string, caseId: string): Promise<void> {
  const query = new URLSearchParams({ case_id: caseId });
  await httpClient.post<{ ok: boolean }>(
    `/api/v1/export/jobs/${encodeURIComponent(jobId)}/cancel?${query.toString()}`,
  );
}
