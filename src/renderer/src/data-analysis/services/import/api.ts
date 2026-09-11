import { httpClient } from "../http/client";
import { requireControlledSourceIngestion } from "../publication-quarantine";

export type JobStatus = "queued" | "running" | "succeeded" | "failed" | "canceled";
export type ImportFileLogView = "active" | "recycle";
export type ImportBatchAction = "recycle" | "restore" | "purge";

export interface PaginationMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
  has_next: boolean;
}

export interface ImportFileSpec {
  file_name: string;
  source_path: string;
  file_kind?: string;
  password?: string;
  expected_sha256?: string;
  expected_size?: number;
  field_mapping?: Record<string, string>;
  field_mapping_origins?: Record<string, string>;
  archive_items?: Array<{
    archive_path: string;
    file_kind?: string;
    expected_sha256: string;
    expected_size: number;
    field_mapping?: Record<string, string>;
    field_mapping_origins?: Record<string, string>;
  }>;
}

export interface ImportJobReq {
  case_id: string;
  files: ImportFileSpec[];
  auto_cleaning: boolean;
}

export interface ImportJobFileDTO {
  file_id: string;
  display_name: string;
  display_path: string;
  file_type: string;
  size: number | null;
  md5: string | null;
  sha256: string | null;
  source_sha256: string | null;
  source_size: number | null;
  kind: string;
  status: string;
  rows_total: number | null;
  rows_seen: number | null;
  rows_imported_raw: number | null;
  rows_imported_norm: number | null;
  rows_dedup: number | null;
  rows_error: number | null;
  rows_skipped_non_data: number | null;
  note: string;
  error: string;
  attempts: number | null;
}

export interface ImportJobDTO {
  job_id: string;
  case_id: string;
  status: JobStatus;
  progress: number;
  imported_files: number | null;
  summary: ImportJobSummaryDTO;
  files: ImportJobFileDTO[];
  current_file: string;
  error: string | null;
  created_at: string;
  updated_at: string;
}

export interface ImportJobSummaryDTO {
  total_files?: number | null;
  rows_total?: number | null;
  rows_seen?: number | null;
  rows_imported_raw?: number | null;
  rows_imported_norm?: number | null;
  rows_dedup?: number | null;
  rows_error?: number | null;
  rows_skipped_non_data?: number | null;
  retry_count?: number | null;
}

export interface ImportJobListData {
  items: ImportJobDTO[];
  page: PaginationMeta;
}

export interface ImportFileLogDTO {
  file_id: string;
  kind: string;
  filename: string;
  display_path: string;
  stored_path: string;
  file_type: string;
  size: number | null;
  md5: string | null;
  sha256: string | null;
  rows_total: number | null;
  rows_imported: number | null;
  rows_imported_raw: number | null;
  rows_imported_norm: number | null;
  rows_dedup: number | null;
  rows_error: number | null;
  rows_skipped_non_data: number | null;
  status: string;
  error: string | null;
  cleaned_status: string | null;
  cleaned_started_at: string | null;
  cleaned_finished_at: string | null;
  cleaned_error: string | null;
  cleaned_rows_affected: number | null;
  created_at: string | null;
  finished_at: string | null;
  recycled_at: string | null;
}

export interface ImportFileLogListData {
  items: ImportFileLogDTO[];
}

export interface ImportHistoricalDatasetDTO {
  dataset_id: string;
  filename: string;
  kind: string;
  rows: number | null;
  cols: number | null;
  imported_at: string | null;
  stored_path: string;
}

export interface ImportHistoricalDatasetListData {
  items: ImportHistoricalDatasetDTO[];
}

export interface ImportBatchDeleteResultDTO {
  ok: boolean;
  deleted_count: number;
  file_ids: string[];
}

export interface ImportBatchActionResultDTO {
  ok: boolean;
  action: ImportBatchAction;
  affected_count: number;
  file_ids: string[];
}

const IMPORT_BATCH_ACTION_RESULT_KEYS = new Set<keyof ImportBatchActionResultDTO>([
  "ok",
  "action",
  "affected_count",
  "file_ids",
]);

export type ImportPreviewDomain = "structured" | "entity" | "support";
export type ImportPreviewStatus = "ready" | "review" | "unsupported";

export interface ImportPreviewArchiveChildDTO {
  file_name: string;
  archive_path: string;
  file_type: string;
  size: number;
  sha256: string;
  rows_total: number;
  columns_total: number;
  header_preview: string[];
  sample_rows: string[][];
  domain_category: ImportPreviewDomain;
  suggested_kind: string;
  suggested_kind_label: string;
  status: ImportPreviewStatus;
  issue: string;
  detected_by: string;
  field_mapping: Record<string, string>;
  field_mapping_origins?: Record<string, string>;
  mapping_status: string;
  mapping_method: string;
  mapping_message: string;
  mapping_required_missing: string[];
}

export interface ImportPreviewFileDTO {
  file_name: string;
  source_path: string;
  file_type: string;
  size: number;
  sha256: string;
  rows_total: number;
  columns_total: number;
  header_preview: string[];
  sample_rows: string[][];
  domain_category: ImportPreviewDomain;
  suggested_kind: string;
  suggested_kind_label: string;
  status: ImportPreviewStatus;
  issue: string;
  accepts_password: boolean;
  requires_password: boolean;
  detected_by: string;
  archive_children: ImportPreviewArchiveChildDTO[];
  field_mapping: Record<string, string>;
  field_mapping_origins?: Record<string, string>;
  mapping_status: string;
  mapping_method: string;
  mapping_message: string;
  mapping_required_missing: string[];
}

export interface ImportPreviewListData {
  items: ImportPreviewFileDTO[];
}

const IMPORT_JOB_STATUS_VALUES = new Set<JobStatus>(["queued", "running", "succeeded", "failed", "canceled"]);
const PUBLIC_IMPORT_FILE_REF = /^importfile_v1_[a-f0-9]{64}$/;
const PUBLIC_IMPORT_DATASET_REF = /^importdataset_v1_[a-f0-9]{64}$/;
const CANONICAL_JOB_ID = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const SAFE_TOKEN = /^[a-z][a-z0-9_]{0,63}$/;
const SAFE_FILE_TYPE = /^[a-z0-9]{1,16}$/;
const LOWER_MD5 = /^[a-f0-9]{32}$/;
const LOWER_SHA256 = /^[a-f0-9]{64}$/;
const ISO_TIMESTAMP = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;
const PUBLIC_SOURCE_NAME = "Imported source";
const IMPORT_FILE_STATUS_VALUES = new Set(["queued", "running", "succeeded", "failed", "canceled", "unknown"]);
const IMPORT_FILE_ERROR_VALUES = new Set([
  "",
  "import_file_processing_failed",
  "import_file_cancelled",
  "import_file_warning",
]);
const IMPORT_FILE_NOTE_VALUES = new Set(["", "import_file_registered", "import_file_warning"]);
const IMPORT_JOB_ERROR_VALUES = new Set([
  "",
  "task_failed",
  "task_canceled",
  "task_interrupted",
  "task_runtime_payload_unavailable",
  "import_job_cancelled",
  "import_job_failed",
]);
const CLEANING_ERROR_VALUES = new Set(["", "cleaning_failed", "cleaning_cancelled"]);
const IMPORT_JOB_KEYS = new Set([
  "job_id",
  "case_id",
  "status",
  "progress",
  "imported_files",
  "summary",
  "files",
  "current_file",
  "error",
  "created_at",
  "updated_at",
]);
const IMPORT_JOB_FILE_KEYS = new Set([
  "file_id",
  "display_name",
  "display_path",
  "file_type",
  "size",
  "md5",
  "sha256",
  "source_sha256",
  "source_size",
  "kind",
  "status",
  "rows_total",
  "rows_seen",
  "rows_imported_raw",
  "rows_imported_norm",
  "rows_dedup",
  "rows_error",
  "rows_skipped_non_data",
  "note",
  "error",
  "attempts",
]);
const IMPORT_JOB_SUMMARY_KEYS = new Set<keyof ImportJobSummaryDTO>([
  "total_files",
  "rows_total",
  "rows_seen",
  "rows_imported_raw",
  "rows_imported_norm",
  "rows_dedup",
  "rows_error",
  "rows_skipped_non_data",
  "retry_count",
]);
const IMPORT_FILE_LOG_KEYS = new Set<keyof ImportFileLogDTO>([
  "file_id",
  "kind",
  "filename",
  "display_path",
  "stored_path",
  "file_type",
  "size",
  "md5",
  "sha256",
  "rows_total",
  "rows_imported",
  "rows_imported_raw",
  "rows_imported_norm",
  "rows_dedup",
  "rows_error",
  "rows_skipped_non_data",
  "status",
  "error",
  "cleaned_status",
  "cleaned_started_at",
  "cleaned_finished_at",
  "cleaned_error",
  "cleaned_rows_affected",
  "created_at",
  "finished_at",
  "recycled_at",
]);
const IMPORT_HISTORICAL_DATASET_KEYS = new Set<keyof ImportHistoricalDatasetDTO>([
  "dataset_id",
  "filename",
  "kind",
  "rows",
  "cols",
  "imported_at",
  "stored_path",
]);

function importJobRecord(value: unknown, field: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`import_job_${field}_invalid`);
  }
  const prototype = Object.getPrototypeOf(value);
  if (prototype !== Object.prototype && prototype !== null) {
    throw new Error(`import_job_${field}_invalid`);
  }
  return value as Record<string, unknown>;
}

function exactImportJobKeys(record: Record<string, unknown>, allowed: ReadonlySet<string>, field: string): void {
  if (Object.keys(record).some((key) => !allowed.has(key))) {
    throw new Error(`import_job_${field}_unknown_field`);
  }
}

function importJobString(
  value: unknown,
  field: string,
  options: { allowEmpty?: boolean } = {},
): string {
  const allowEmpty = options.allowEmpty ?? true;
  if (typeof value !== "string" || (!allowEmpty && value.length === 0)) {
    throw new Error(`import_job_${field}_invalid`);
  }
  return value;
}

function requirePublicImportRef(value: unknown, field: string): string {
  const result = importJobString(value, field, { allowEmpty: false });
  if (!PUBLIC_IMPORT_FILE_REF.test(result)) {
    throw new Error(`import_job_${field}_invalid`);
  }
  return result;
}

function requireExactString(value: unknown, expected: string, field: string): string {
  const result = importJobString(value, field);
  if (result !== expected) {
    throw new Error(`import_job_${field}_invalid`);
  }
  return result;
}

function requireSafeToken(value: unknown, field: string, allowEmpty = true): string {
  const result = importJobString(value, field);
  if ((result === "" && allowEmpty) || SAFE_TOKEN.test(result)) {
    return result;
  }
  throw new Error(`import_job_${field}_invalid`);
}

function requireSafeFileType(value: unknown, field: string): string {
  const result = importJobString(value, field);
  if (result === "" || SAFE_FILE_TYPE.test(result)) {
    return result;
  }
  throw new Error(`import_job_${field}_invalid`);
}

function requireNullableSafeHash(
  value: unknown,
  field: string,
  pattern: RegExp,
): string | null {
  if (value === null) return null;
  const result = importJobString(value, field, { allowEmpty: false });
  if (pattern.test(result)) {
    return result;
  }
  throw new Error(`import_job_${field}_invalid`);
}

function requireAllowedString(
  value: unknown,
  field: string,
  allowed: ReadonlySet<string>,
): string {
  const result = importJobString(value, field);
  if (!allowed.has(result)) {
    throw new Error(`import_job_${field}_invalid`);
  }
  return result;
}

function requireTimestamp(value: unknown, field: string): string {
  const result = importJobString(value, field, { allowEmpty: false });
  if (!ISO_TIMESTAMP.test(result) || !Number.isFinite(Date.parse(result))) {
    throw new Error(`import_job_${field}_invalid`);
  }
  return result;
}

function importJobUnsigned(value: unknown, field: string): number {
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    Object.is(value, -0)
  ) {
    throw new Error(`import_job_${field}_invalid`);
  }
  return value;
}

function importJobNullableUnsigned(value: unknown, field: string): number | null {
  return value === null ? null : importJobUnsigned(value, field);
}

function importFileLogRecord(value: unknown, field: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`import_file_log_${field}_invalid`);
  }
  const prototype = Object.getPrototypeOf(value);
  if (prototype !== Object.prototype && prototype !== null) {
    throw new Error(`import_file_log_${field}_invalid`);
  }
  return value as Record<string, unknown>;
}

function importFileLogString(value: unknown, field: string): string {
  if (typeof value !== "string") {
    throw new Error(`import_file_log_${field}_invalid`);
  }
  return value;
}

function importFileLogNullableString(value: unknown, field: string): string | null {
  return value === null ? null : importFileLogString(value, field);
}

function importFileLogNullableHash(value: unknown, field: string, pattern: RegExp): string | null {
  if (value === null) return null;
  const result = importFileLogString(value, field);
  if (!pattern.test(result)) {
    throw new Error(`import_file_log_${field}_invalid`);
  }
  return result;
}

function importFileLogNullableTimestamp(value: unknown, field: string): string | null {
  if (value === null) return null;
  const result = importFileLogString(value, field);
  if (!ISO_TIMESTAMP.test(result) || !Number.isFinite(Date.parse(result))) {
    throw new Error(`import_file_log_${field}_invalid`);
  }
  return result;
}

function importFileLogNullableCount(value: unknown, field: string): number | null {
  if (value === null) {
    return null;
  }
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    Object.is(value, -0)
  ) {
    throw new Error(`import_file_log_${field}_invalid`);
  }
  return value;
}

export function parseImportFileLogDTO(value: unknown): ImportFileLogDTO {
  const record = importFileLogRecord(value, "item");
  if (Object.keys(record).some((key) => !IMPORT_FILE_LOG_KEYS.has(key as keyof ImportFileLogDTO))) {
    throw new Error("import_file_log_item_unknown_field");
  }
  for (const key of IMPORT_FILE_LOG_KEYS) {
    if (!Object.prototype.hasOwnProperty.call(record, key)) {
      throw new Error(`import_file_log_${key}_missing`);
    }
  }
  const fileId = importFileLogString(record.file_id, "file_id");
  const kind = importFileLogString(record.kind, "kind");
  const filename = importFileLogString(record.filename, "filename");
  const displayPath = importFileLogString(record.display_path, "display_path");
  const storedPath = importFileLogString(record.stored_path, "stored_path");
  const fileType = importFileLogString(record.file_type, "file_type");
  const md5 = importFileLogNullableHash(record.md5, "md5", LOWER_MD5);
  const sha256 = importFileLogNullableHash(record.sha256, "sha256", LOWER_SHA256);
  const status = importFileLogString(record.status, "status");
  const error = importFileLogNullableString(record.error, "error");
  const cleanedStatus = importFileLogNullableString(record.cleaned_status, "cleaned_status");
  const cleanedError = importFileLogNullableString(record.cleaned_error, "cleaned_error");
  if (!PUBLIC_IMPORT_FILE_REF.test(fileId)) throw new Error("import_file_log_file_id_invalid");
  if (kind !== "" && !SAFE_TOKEN.test(kind)) throw new Error("import_file_log_kind_invalid");
  if (filename !== PUBLIC_SOURCE_NAME) throw new Error("import_file_log_filename_invalid");
  if (displayPath !== "" || storedPath !== "") throw new Error("import_file_log_path_invalid");
  if (fileType !== "" && !SAFE_FILE_TYPE.test(fileType)) throw new Error("import_file_log_file_type_invalid");
  if (!IMPORT_FILE_STATUS_VALUES.has(status)) throw new Error("import_file_log_status_invalid");
  if (error !== null && !IMPORT_FILE_ERROR_VALUES.has(error)) throw new Error("import_file_log_error_invalid");
  if (cleanedStatus !== null && !IMPORT_FILE_STATUS_VALUES.has(cleanedStatus)) {
    throw new Error("import_file_log_cleaned_status_invalid");
  }
  if (cleanedError !== null && !CLEANING_ERROR_VALUES.has(cleanedError)) {
    throw new Error("import_file_log_cleaned_error_invalid");
  }
  return {
    file_id: fileId,
    kind,
    filename,
    display_path: displayPath,
    stored_path: storedPath,
    file_type: fileType,
    size: importFileLogNullableCount(record.size, "size"),
    md5,
    sha256,
    rows_total: importFileLogNullableCount(record.rows_total, "rows_total"),
    rows_imported: importFileLogNullableCount(record.rows_imported, "rows_imported"),
    rows_imported_raw: importFileLogNullableCount(record.rows_imported_raw, "rows_imported_raw"),
    rows_imported_norm: importFileLogNullableCount(record.rows_imported_norm, "rows_imported_norm"),
    rows_dedup: importFileLogNullableCount(record.rows_dedup, "rows_dedup"),
    rows_error: importFileLogNullableCount(record.rows_error, "rows_error"),
    rows_skipped_non_data: importFileLogNullableCount(
      record.rows_skipped_non_data,
      "rows_skipped_non_data",
    ),
    status,
    error,
    cleaned_status: cleanedStatus,
    cleaned_started_at: importFileLogNullableTimestamp(record.cleaned_started_at, "cleaned_started_at"),
    cleaned_finished_at: importFileLogNullableTimestamp(record.cleaned_finished_at, "cleaned_finished_at"),
    cleaned_error: cleanedError,
    cleaned_rows_affected: importFileLogNullableCount(
      record.cleaned_rows_affected,
      "cleaned_rows_affected",
    ),
    created_at: importFileLogNullableTimestamp(record.created_at, "created_at"),
    finished_at: importFileLogNullableTimestamp(record.finished_at, "finished_at"),
    recycled_at: importFileLogNullableTimestamp(record.recycled_at, "recycled_at"),
  };
}

export function parseImportFileLogListData(value: unknown): ImportFileLogListData {
  const record = importFileLogRecord(value, "list");
  if (Object.keys(record).some((key) => key !== "items")) {
    throw new Error("import_file_log_list_unknown_field");
  }
  if (!Array.isArray(record.items)) {
    throw new Error("import_file_log_list_items_invalid");
  }
  return { items: record.items.map(parseImportFileLogDTO) };
}

function historicalDatasetRecord(value: unknown, field: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`historical_dataset_${field}_invalid`);
  }
  const prototype = Object.getPrototypeOf(value);
  if (prototype !== Object.prototype && prototype !== null) {
    throw new Error(`historical_dataset_${field}_invalid`);
  }
  return value as Record<string, unknown>;
}

function historicalDatasetString(value: unknown, field: string): string {
  if (typeof value !== "string") {
    throw new Error(`historical_dataset_${field}_invalid`);
  }
  return value;
}

function historicalDatasetNullableString(value: unknown, field: string): string | null {
  return value === null ? null : historicalDatasetString(value, field);
}

function historicalDatasetNullableTimestamp(value: unknown, field: string): string | null {
  if (value === null) return null;
  const result = historicalDatasetString(value, field);
  if (!ISO_TIMESTAMP.test(result) || !Number.isFinite(Date.parse(result))) {
    throw new Error(`historical_dataset_${field}_invalid`);
  }
  return result;
}

function historicalDatasetNullableCount(value: unknown, field: string): number | null {
  if (value === null) {
    return null;
  }
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    Object.is(value, -0)
  ) {
    throw new Error(`historical_dataset_${field}_invalid`);
  }
  return value;
}

export function parseImportHistoricalDatasetDTO(value: unknown): ImportHistoricalDatasetDTO {
  const record = historicalDatasetRecord(value, "item");
  if (
    Object.keys(record).some(
      (key) => !IMPORT_HISTORICAL_DATASET_KEYS.has(key as keyof ImportHistoricalDatasetDTO),
    )
  ) {
    throw new Error("historical_dataset_item_unknown_field");
  }
  for (const key of IMPORT_HISTORICAL_DATASET_KEYS) {
    if (!Object.prototype.hasOwnProperty.call(record, key)) {
      throw new Error(`historical_dataset_${key}_missing`);
    }
  }
  const datasetId = historicalDatasetString(record.dataset_id, "dataset_id");
  const filename = historicalDatasetString(record.filename, "filename");
  const kind = historicalDatasetString(record.kind, "kind");
  const storedPath = historicalDatasetString(record.stored_path, "stored_path");
  if (!PUBLIC_IMPORT_DATASET_REF.test(datasetId)) throw new Error("historical_dataset_dataset_id_invalid");
  if (filename !== PUBLIC_SOURCE_NAME) throw new Error("historical_dataset_filename_invalid");
  if (kind !== "" && !SAFE_TOKEN.test(kind)) throw new Error("historical_dataset_kind_invalid");
  if (storedPath !== "") throw new Error("historical_dataset_stored_path_invalid");
  return {
    dataset_id: datasetId,
    filename,
    kind,
    rows: historicalDatasetNullableCount(record.rows, "rows"),
    cols: historicalDatasetNullableCount(record.cols, "cols"),
    imported_at: historicalDatasetNullableTimestamp(record.imported_at, "imported_at"),
    stored_path: storedPath,
  };
}

export function parseImportHistoricalDatasetListData(
  value: unknown,
): ImportHistoricalDatasetListData {
  const record = historicalDatasetRecord(value, "list");
  if (Object.keys(record).some((key) => key !== "items")) {
    throw new Error("historical_dataset_list_unknown_field");
  }
  if (!Object.prototype.hasOwnProperty.call(record, "items")) {
    throw new Error("historical_dataset_list_items_missing");
  }
  if (!Array.isArray(record.items)) {
    throw new Error("historical_dataset_list_items_invalid");
  }
  return { items: record.items.map(parseImportHistoricalDatasetDTO) };
}

export function parseImportBatchActionResultDTO(
  value: unknown,
  expectedAction: ImportBatchAction,
  requestedFileIds: readonly string[],
): ImportBatchActionResultDTO {
  const record = historicalDatasetRecord(value, "batch_action");
  if (
    Object.keys(record).some(
      (key) => !IMPORT_BATCH_ACTION_RESULT_KEYS.has(key as keyof ImportBatchActionResultDTO),
    )
  ) {
    throw new Error("import_batch_action_unknown_field");
  }
  for (const key of IMPORT_BATCH_ACTION_RESULT_KEYS) {
    if (!Object.prototype.hasOwnProperty.call(record, key)) {
      throw new Error(`import_batch_action_${key}_missing`);
    }
  }
  if (record.ok !== true) {
    throw new Error("import_batch_action_not_successful");
  }
  if (record.action !== expectedAction) {
    throw new Error("import_batch_action_mismatch");
  }
  const affectedCount = historicalDatasetNullableCount(record.affected_count, "affected_count");
  if (affectedCount === null) {
    throw new Error("import_batch_action_affected_count_invalid");
  }
  if (!Array.isArray(record.file_ids)) {
    throw new Error("import_batch_action_file_ids_invalid");
  }
  const requested = new Set(requestedFileIds);
  const fileIds = record.file_ids.map((value) => {
    if (typeof value !== "string" || value.trim() !== value || value.length === 0 || !requested.has(value)) {
      throw new Error("import_batch_action_file_id_invalid");
    }
    return value;
  });
  if (new Set(fileIds).size !== fileIds.length || affectedCount !== fileIds.length) {
    throw new Error("import_batch_action_coverage_mismatch");
  }
  return { ok: true, action: expectedAction, affected_count: affectedCount, file_ids: fileIds };
}

function parseImportJobSummaryDTO(value: unknown): ImportJobSummaryDTO {
  const record = importJobRecord(value, "summary");
  exactImportJobKeys(record, IMPORT_JOB_SUMMARY_KEYS, "summary");
  const summary: ImportJobSummaryDTO = {};
  for (const key of IMPORT_JOB_SUMMARY_KEYS) {
    if (Object.prototype.hasOwnProperty.call(record, key)) {
      summary[key] = importJobNullableUnsigned(record[key], `summary_${key}`);
    }
  }
  return summary;
}

function parseImportJobFileDTO(value: unknown): ImportJobFileDTO {
  const record = importJobRecord(value, "file");
  exactImportJobKeys(record, IMPORT_JOB_FILE_KEYS, "file");
  for (const key of IMPORT_JOB_FILE_KEYS) {
    if (!Object.prototype.hasOwnProperty.call(record, key)) {
      throw new Error(`import_job_file_${key}_missing`);
    }
  }
  const status = requireAllowedString(record.status, "file_status", IMPORT_FILE_STATUS_VALUES);
  return {
    file_id: requirePublicImportRef(record.file_id, "file_id"),
    display_name: requireExactString(record.display_name, PUBLIC_SOURCE_NAME, "file_display_name"),
    display_path: requireExactString(record.display_path, "", "file_display_path"),
    file_type: requireSafeFileType(record.file_type, "file_type"),
    size: importJobNullableUnsigned(record.size, "file_size"),
    md5: requireNullableSafeHash(record.md5, "file_md5", LOWER_MD5),
    sha256: requireNullableSafeHash(record.sha256, "file_sha256", LOWER_SHA256),
    source_sha256: requireNullableSafeHash(record.source_sha256, "file_source_sha256", LOWER_SHA256),
    source_size: importJobNullableUnsigned(record.source_size, "file_source_size"),
    kind: requireSafeToken(record.kind, "file_kind"),
    status,
    rows_total: importJobNullableUnsigned(record.rows_total, "file_rows_total"),
    rows_seen: importJobNullableUnsigned(record.rows_seen, "file_rows_seen"),
    rows_imported_raw: importJobNullableUnsigned(record.rows_imported_raw, "file_rows_imported_raw"),
    rows_imported_norm: importJobNullableUnsigned(record.rows_imported_norm, "file_rows_imported_norm"),
    rows_dedup: importJobNullableUnsigned(record.rows_dedup, "file_rows_dedup"),
    rows_error: importJobNullableUnsigned(record.rows_error, "file_rows_error"),
    rows_skipped_non_data: importJobNullableUnsigned(record.rows_skipped_non_data, "file_rows_skipped_non_data"),
    note: requireAllowedString(record.note, "file_note", IMPORT_FILE_NOTE_VALUES),
    error: requireAllowedString(record.error, "file_error", IMPORT_FILE_ERROR_VALUES),
    attempts: importJobNullableUnsigned(record.attempts, "file_attempts"),
  };
}

export function parseImportJobDTO(value: unknown, expectedCaseId: string): ImportJobDTO {
  const record = importJobRecord(value, "payload");
  exactImportJobKeys(record, IMPORT_JOB_KEYS, "payload");
  for (const key of IMPORT_JOB_KEYS) {
    if (!Object.prototype.hasOwnProperty.call(record, key)) {
      throw new Error(`import_job_${key}_missing`);
    }
  }
  const status = importJobString(record.status, "status") as JobStatus;
  if (!IMPORT_JOB_STATUS_VALUES.has(status)) {
    throw new Error("import_job_status_invalid");
  }
  const progress = importJobUnsigned(record.progress, "progress");
  if (progress > 100) {
    throw new Error("import_job_progress_invalid");
  }
  if (!Array.isArray(record.files)) {
    throw new Error("import_job_files_invalid");
  }
  const error = record.error === null ? null : importJobString(record.error, "error");
  const jobId = importJobString(record.job_id, "job_id", { allowEmpty: false });
  const caseId = importJobString(record.case_id, "case_id", { allowEmpty: false });
  if (!CANONICAL_JOB_ID.test(jobId)) throw new Error("import_job_job_id_invalid");
  if (caseId !== expectedCaseId) throw new Error("import_job_case_id_mismatch");
  if (error !== null && !IMPORT_JOB_ERROR_VALUES.has(error)) {
    throw new Error("import_job_error_invalid");
  }
  const currentFile = importJobString(record.current_file, "current_file");
  if (currentFile !== "") throw new Error("import_job_current_file_invalid");
  return {
    job_id: jobId,
    case_id: caseId,
    status,
    progress,
    imported_files: importJobNullableUnsigned(record.imported_files, "imported_files"),
    summary: parseImportJobSummaryDTO(record.summary),
    files: record.files.map(parseImportJobFileDTO),
    current_file: currentFile,
    error,
    created_at: requireTimestamp(record.created_at, "created_at"),
    updated_at: requireTimestamp(record.updated_at, "updated_at"),
  };
}

function parseImportJobListData(value: unknown, expectedCaseId: string): ImportJobListData {
  const record = importJobRecord(value, "list");
  exactImportJobKeys(record, new Set(["items", "page"]), "list");
  if (!Array.isArray(record.items)) {
    throw new Error("import_job_list_items_invalid");
  }
  const page = importJobRecord(record.page, "page");
  exactImportJobKeys(
    page,
    new Set(["page", "page_size", "total", "total_pages", "has_next"]),
    "page",
  );
  if (typeof page.has_next !== "boolean") {
    throw new Error("import_job_page_has_next_invalid");
  }
  return {
    items: record.items.map((item) => parseImportJobDTO(item, expectedCaseId)),
    page: {
      page: importJobUnsigned(page.page, "page_page"),
      page_size: importJobUnsigned(page.page_size, "page_size"),
      total: importJobUnsigned(page.total, "page_total"),
      total_pages: importJobUnsigned(page.total_pages, "page_total_pages"),
      has_next: page.has_next,
    },
  };
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

export async function createImportJob(payload: ImportJobReq): Promise<ImportJobDTO> {
  requireControlledSourceIngestion();
  const response = await httpClient.post<ImportJobDTO>("/api/v1/import/jobs", payload);
  return parseImportJobDTO(response.data, payload.case_id);
}

export async function listImportJobs(params: {
  caseId: string;
  page?: number;
  pageSize?: number;
  status?: JobStatus | "";
}): Promise<ImportJobListData> {
  const query = buildJobsQuery(params);
  const response = await httpClient.get<ImportJobListData>(`/api/v1/import/jobs?${query}`);
  return parseImportJobListData(response.data, params.caseId);
}

export async function getImportJob(caseId: string, jobId: string): Promise<ImportJobDTO> {
  const query = new URLSearchParams({ case_id: caseId });
  const response = await httpClient.get<ImportJobDTO>(
    `/api/v1/import/jobs/${encodeURIComponent(jobId)}?${query.toString()}`,
  );
  return parseImportJobDTO(response.data, caseId);
}

export async function cancelImportJob(caseId: string, jobId: string): Promise<void> {
  const query = new URLSearchParams({ case_id: caseId });
  await httpClient.post<{ ok: boolean }>(
    `/api/v1/import/jobs/${encodeURIComponent(jobId)}/cancel?${query.toString()}`,
  );
}

export async function listImportFiles(caseId: string, view: ImportFileLogView = "active"): Promise<ImportFileLogListData> {
  const query = new URLSearchParams();
  query.set("case_id", caseId);
  query.set("view", view);
  const response = await httpClient.get<unknown>(`/api/v1/import/files?${query.toString()}`);
  return parseImportFileLogListData(response.data);
}

export async function listHistoricalImportDatasets(caseId: string): Promise<ImportHistoricalDatasetListData> {
  const query = new URLSearchParams();
  query.set("case_id", caseId);
  const response = await httpClient.get<unknown>(
    `/api/v1/import/files/historical-datasets?${query.toString()}`
  );
  return parseImportHistoricalDatasetListData(response.data);
}

export async function previewImportFiles(caseId: string, files: ImportFileSpec[]): Promise<ImportPreviewListData> {
  requireControlledSourceIngestion();
  const response = await httpClient.post<ImportPreviewListData>("/api/v1/import/files/preview", {
    case_id: caseId,
    files
  });
  return response.data;
}

export async function deleteImportFile(caseId: string, fileId: string): Promise<void> {
  const query = new URLSearchParams();
  query.set("case_id", caseId);
  await httpClient.delete<{ ok: boolean }>(`/api/v1/import/files/${encodeURIComponent(fileId)}?${query.toString()}`);
}

export async function deleteImportFiles(caseId: string, fileIds: string[]): Promise<ImportBatchDeleteResultDTO> {
  const response = await httpClient.post<ImportBatchDeleteResultDTO>("/api/v1/import/files/delete-batch", {
    case_id: caseId,
    file_ids: fileIds
  });
  return response.data;
}

export async function recycleImportFiles(caseId: string, fileIds: string[]): Promise<ImportBatchActionResultDTO> {
  const response = await httpClient.post<unknown>("/api/v1/import/files/recycle-batch", {
    case_id: caseId,
    file_ids: fileIds
  });
  return parseImportBatchActionResultDTO(response.data, "recycle", fileIds);
}

export async function restoreImportFiles(caseId: string, fileIds: string[]): Promise<ImportBatchActionResultDTO> {
  const response = await httpClient.post<unknown>("/api/v1/import/files/restore-batch", {
    case_id: caseId,
    file_ids: fileIds
  });
  return parseImportBatchActionResultDTO(response.data, "restore", fileIds);
}

export async function purgeImportFiles(caseId: string, fileIds: string[]): Promise<ImportBatchActionResultDTO> {
  const response = await httpClient.post<unknown>("/api/v1/import/files/purge-batch", {
    case_id: caseId,
    file_ids: fileIds
  });
  return parseImportBatchActionResultDTO(response.data, "purge", fileIds);
}
