import type { ImportPreviewFileDTO, JobStatus } from "../api";

export type SortField = "created" | "status" | "kind" | "name" | "rows" | "size";
export type SortDirection = "asc" | "desc";
export type ImportStatusFilter = "all" | "active" | "success" | "attention";
export type ImportKindFilter =
  | "all"
  | "__tasks__"
  | "fc_account"
  | "fc_person"
  | "fc_coercive_measure"
  | "fc_transaction"
  | "fc_sub_account"
  | "fc_person_address"
  | "fc_person_contact"
  | "fc_task_success"
  | "fc_task_fail";
export type WizardStep = "select" | "mapping" | "validation" | "summary";
export type WizardFileState = "ready" | "review" | "unsupported" | "running" | "succeeded" | "failed";
export type ImportStudioPhase = "idle" | "staged" | "review" | "importing" | "completed";
export type WizardPasswordState = "idle" | "validating" | "verified" | "error";
export type WizardProgressFilter = "all" | "completed" | "pending";
export type MappingFieldOrigin = "manual" | "auto" | "ai" | "empty" | "info";
export type ImportDomainCategory = "all" | "structured" | "entity" | "support";
export type MappingFieldState = "matched" | "required" | "suggested" | "unresolved" | "info";
export type MappingWorkbenchFilter = "all" | "pending" | "required" | "auto";
export type MappingWorkbenchReasonTone = "success" | "manual" | "ai" | "warn" | "muted";

export interface LedgerRow {
  key: string;
  file_id: string;
  display_name: string;
  display_path: string;
  stored_path: string;
  file_type: string;
  size: number | null;
  md5: string;
  sha256: string;
  kind: string;
  kind_label: string;
  status_text: string;
  rows_total: number | null;
  rows_seen: number | null;
  rows_imported_raw: number | null;
  rows_imported_norm: number | null;
  rows_dedup: number | null;
  rows_error: number | null;
  rows_skipped_non_data: number | null;
  valid_rows: number | null;
  duplicate_rows: number | null;
  note: string;
  error: string;
  created_at: string;
  finished_at: string;
  recycled_at: string;
  attempts: number | null;
}

export interface ImportWizardFile {
  id: string;
  fileName: string;
  sourcePath: string;
  fileType: string;
  size: number;
  rowsTotal: number;
  columnsTotal: number;
  headerPreview: string[];
  sampleRows: string[][];
  domainCategory: Exclude<ImportDomainCategory, "all">;
  selectedCategory: Exclude<ImportDomainCategory, "all">;
  suggestedKind: string;
  selectedKind: string;
  suggestedKindLabel: string;
  status: WizardFileState;
  issue: string;
  acceptsPassword: boolean;
  requiresPassword: boolean;
  password: string;
  passwordState: WizardPasswordState;
  passwordVerified: boolean;
  passwordValidationMessage: string;
  fieldMapping: Record<string, string>;
  fieldMappingOrigins: Record<string, MappingFieldOrigin>;
  mappingStatus: string;
  mappingMethod: string;
  mappingMessage: string;
  mappingRequiredMissing: string[];
  progress: number;
  detectedBy: string;
  sha256: string;
  archiveChildren: ImportWizardArchiveChild[];
}

export interface ImportWizardArchiveChild {
  id: string;
  fileName: string;
  archivePath: string;
  fileType: string;
  size: number;
  rowsTotal: number;
  columnsTotal: number;
  headerPreview: string[];
  sampleRows: string[][];
  domainCategory: Exclude<ImportDomainCategory, "all">;
  suggestedKind: string;
  selectedKind: string;
  suggestedKindLabel: string;
  status: WizardFileState;
  issue: string;
  detectedBy: string;
  sha256: string;
  fieldMapping: Record<string, string>;
  fieldMappingOrigins: Record<string, MappingFieldOrigin>;
  mappingStatus: string;
  mappingMethod: string;
  mappingMessage: string;
  mappingRequiredMissing: string[];
}

export interface ImportWizardProgressChild extends ImportWizardArchiveChild {
  progress: number;
  kindLabel: string;
  isActive: boolean;
}

export interface ImportWizardProgressGroup extends ImportWizardFile {
  isArchive: boolean;
  childProgress: ImportWizardProgressChild[];
  isActive: boolean;
  message: string;
}

export interface MappingFieldBlueprint {
  key: string;
  label: string;
  aliases: string[];
  required?: boolean;
  importHeader?: string;
}

export interface MappingSourceFieldPreview {
  key: string;
  label: string;
  state: Exclude<MappingFieldState, "required">;
  targetLabel: string;
}

export interface MappingTargetFieldPreview {
  key: string;
  label: string;
  importHeader: string;
  state: Exclude<MappingFieldState, "unresolved">;
  required: boolean;
  sourceLabel: string;
  origin: MappingFieldOrigin;
}

export interface MappingInsight {
  summary: string;
  headerCount: number;
  matchedRequired: number;
  requiredTotal: number;
  matchedTotal: number;
  unresolvedCount: number;
  riskCount: number;
  confidenceLabel: string;
  sourceFields: MappingSourceFieldPreview[];
  targetFields: MappingTargetFieldPreview[];
}

export interface MappingWorkbenchColumn {
  header: string;
  targetKey: string;
  targetHeader: string;
  targetFieldLabel: string;
  targetLabel: string;
  targetRequired: boolean;
  origin: MappingFieldOrigin | "empty";
  state: Exclude<MappingFieldState, "required">;
  sampleValues: string[];
  matchBasis: string;
  matchBasisTone: MappingWorkbenchReasonTone;
}

export interface MappingAssistState {
  status: "idle" | "running" | "done" | "error";
  message: string;
}

export interface WizardStepOption {
  value: WizardStep;
  label: string;
  english: string;
}

export const KIND_CN: Record<string, string> = {
  fc_account: "账户信息",
  fc_person: "人员信息",
  fc_coercive_measure: "强制措施信息",
  fc_transaction: "交易明细信息",
  fc_sub_account: "关联子账户信息",
  fc_person_address: "人员住址信息",
  fc_person_contact: "人员联系方式信息",
  fc_task_success: "任务信息(成功)",
  fc_task_fail: "任务信息(失败)",
  support_file: "研判文件",
  __tasks__: "任务数"
};

export const TILE_ORDER = [
  "__tasks__",
  "fc_account",
  "fc_person",
  "fc_coercive_measure",
  "fc_transaction",
  "fc_sub_account",
  "fc_person_address",
  "fc_person_contact",
  "fc_task_success",
  "fc_task_fail"
];

export const SORT_FIELD_OPTIONS: Array<{ value: SortField; label: string }> = [
  { value: "created", label: "创建日期" },
  { value: "status", label: "状态" },
  { value: "kind", label: "类型" },
  { value: "name", label: "文件名" },
  { value: "rows", label: "行数" },
  { value: "size", label: "大小" }
];

export const SORT_DIRECTION_OPTIONS: Array<{ value: SortDirection; label: string }> = [
  { value: "desc", label: "降序" },
  { value: "asc", label: "升序" }
];

export const STATUS_FILTER_OPTIONS: Array<{ value: ImportStatusFilter; label: string }> = [
  { value: "all", label: "全部台账" },
  { value: "active", label: "进行中" },
  { value: "success", label: "已完成" },
  { value: "attention", label: "关注项" }
];

export const EXPORT_TABLE_LABELS: Record<string, string> = {
  fc_coercive_measure: "强制措施信息",
  fc_person_contact: "人员联系方式信息",
  fc_person_address: "人员住址信息",
  fc_person: "人员信息",
  fc_sub_account: "关联子账户信息",
  fc_account: "账户信息",
  fc_transaction: "交易明细信息",
  fc_task_fail: "任务信息(失败)",
  fc_task_success: "任务信息(成功)"
};

export const WIZARD_STEPS: WizardStepOption[] = [
  { value: "select", label: "文件预检", english: "Precheck" },
  { value: "mapping", label: "字段确认", english: "Mapping" },
  { value: "validation", label: "执行入库", english: "Execute" },
  { value: "summary", label: "完成导入", english: "Summary" }
];

export const CATEGORY_KIND_OPTIONS: Record<Exclude<ImportDomainCategory, "all">, Array<{ value: string; label: string }>> = {
  structured: [
    { value: "fc_account", label: "账户信息" },
    { value: "fc_transaction", label: "交易明细信息" },
    { value: "fc_sub_account", label: "关联子账户信息" },
    { value: "fc_coercive_measure", label: "强制措施信息" },
    { value: "fc_task_success", label: "任务信息(成功)" },
    { value: "fc_task_fail", label: "任务信息(失败)" }
  ],
  entity: [
    { value: "fc_person", label: "人员信息" },
    { value: "fc_person_address", label: "人员住址信息" },
    { value: "fc_person_contact", label: "人员联系方式信息" }
  ],
  support: [{ value: "support_file", label: "研判文件" }]
};

const EXECUTION_OVERVIEW_FILE_NAME_MAX_LENGTH = 18;
const EXECUTION_OVERVIEW_FILE_NAME_HEAD_LIMIT = 10;
const EXECUTION_OVERVIEW_FILE_NAME_TAIL_LIMIT = 10;
const ORGANIZATION_NAME_SUFFIXES = [
  "集团有限责任公司",
  "集团股份有限公司",
  "集团有限公司",
  "股份有限公司",
  "有限责任公司",
  "有限公司",
  "股份公司"
];

export function isTerminalStatus(status: JobStatus): boolean {
  return status === "succeeded" || status === "failed" || status === "canceled";
}

export function mapEventToStatus(event: string, previous: JobStatus, payload: Record<string, unknown>): JobStatus {
  if (isTerminalStatus(previous)) {
    return previous;
  }
  if (event.endsWith("queued")) {
    return "queued";
  }
  if (event.endsWith("progress")) {
    return "running";
  }
  if (event.endsWith("completed")) {
    return "succeeded";
  }
  if (event.endsWith("failed")) {
    if (String(payload.code ?? "").trim() === "JOB_CANCELED") {
      return "canceled";
    }
    return "failed";
  }
  return previous;
}

export function fileNameFromPath(pathValue: string): string {
  const normalized = String(pathValue || "").replace(/\\/g, "/");
  const parts = normalized.split("/").filter(Boolean);
  return parts[parts.length - 1] ?? "";
}

export function splitFileNameParts(fileName: string): { baseName: string; extension: string } {
  const trimmed = String(fileName || "").trim();
  const match = /(\.[^.]+)$/.exec(trimmed);
  if (!match) {
    return {
      baseName: trimmed,
      extension: ""
    };
  }
  return {
    baseName: trimmed.slice(0, -match[1].length),
    extension: match[1]
  };
}

function stripOrganizationSuffix(value: string): string {
  const normalized = String(value || "").trim().replace(/[()（）._\-\s]+$/g, "");
  for (const suffix of ORGANIZATION_NAME_SUFFIXES) {
    if (!normalized.endsWith(suffix)) {
      continue;
    }
    const stripped = normalized.slice(0, -suffix.length).trim();
    if (stripped.length >= 4) {
      return stripped;
    }
  }
  return normalized;
}

function detectExecutionFileTail(baseName: string, typeHint = ""): string {
  const normalizedBase = String(baseName || "").trim();
  if (!normalizedBase) {
    return "";
  }

  const candidates = Array.from(new Set([String(typeHint || "").trim(), ...Object.values(KIND_CN)]))
    .filter((candidate) => candidate && candidate !== "任务数")
    .sort((left, right) => right.length - left.length);

  for (const candidate of candidates) {
    const index = normalizedBase.lastIndexOf(candidate);
    if (index < 0) {
      continue;
    }
    if (normalizedBase.endsWith(candidate)) {
      return candidate;
    }
    if (normalizedBase.length - index <= candidate.length + 6) {
      return normalizedBase.slice(index);
    }
  }

  return "";
}

export function formatExecutionOverviewFileName(fileName: string, typeHint = ""): string {
  const trimmed = String(fileName || "").trim();
  if (!trimmed || trimmed.length <= EXECUTION_OVERVIEW_FILE_NAME_MAX_LENGTH) {
    return trimmed || "未命名文件";
  }

  const { baseName, extension } = splitFileNameParts(trimmed);
  const tail = detectExecutionFileTail(baseName, typeHint);

  if (tail) {
    const prefixBase = baseName.slice(0, Math.max(0, baseName.lastIndexOf(tail)));
    const normalizedPrefix = stripOrganizationSuffix(prefixBase) || prefixBase.trim();
    const visiblePrefix = (normalizedPrefix || baseName).slice(0, EXECUTION_OVERVIEW_FILE_NAME_HEAD_LIMIT).trim();
    const compactName = `${visiblePrefix}...${tail}${extension}`;
    if (compactName.length < trimmed.length) {
      return compactName;
    }
  }

  if (!extension) {
    return trimmed;
  }

  const head = baseName.slice(0, EXECUTION_OVERVIEW_FILE_NAME_HEAD_LIMIT).trim();
  const tailFallback = baseName.slice(-EXECUTION_OVERVIEW_FILE_NAME_TAIL_LIMIT).trim();
  if (!head || !tailFallback) {
    return trimmed;
  }

  const compactName = `${head}...${tailFallback}${extension}`;
  return compactName.length < trimmed.length ? compactName : trimmed;
}

export function toInt(value: unknown): number {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n)) {
    return 0;
  }
  return Math.max(0, Math.trunc(n));
}

export function clampNumber(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export function normalizeKind(rawKind: string, displayName = ""): string {
  const kind = String(rawKind || "").trim();
  const name = String(displayName || "");
  if (name.includes("子账户") && (kind === "" || kind === "fc_account")) {
    return "fc_sub_account";
  }
  if (!kind) {
    return "";
  }
  if (KIND_CN[kind]) {
    return kind;
  }
  for (const [key, label] of Object.entries(KIND_CN)) {
    if (kind === label) {
      return key;
    }
  }
  return kind;
}

export function kindLabel(kind: string): string {
  return KIND_CN[kind] ?? kind ?? "—";
}

export function resolveImportCategory(kind: string, fileName = "", filePath = ""): Exclude<ImportDomainCategory, "all"> {
  const normalizedKind = normalizeKind(kind, fileName);
  if (normalizedKind === "fc_person" || normalizedKind === "fc_person_address" || normalizedKind === "fc_person_contact") {
    return "entity";
  }
  if (normalizedKind.startsWith("fc_")) {
    return "structured";
  }

  const blob = `${fileName} ${filePath}`.toLowerCase();
  if (/\.(pdf|doc|docx|txt|md|json|xml|png|jpg|jpeg|zip)$/i.test(blob) || /资料|材料|附件|报告/.test(blob)) {
    return "support";
  }
  return "support";
}

export function categoryKindOptions(category: Exclude<ImportDomainCategory, "all">): Array<{ value: string; label: string }> {
  return CATEGORY_KIND_OPTIONS[category] ?? CATEGORY_KIND_OPTIONS.support;
}

export function defaultKindForCategory(category: Exclude<ImportDomainCategory, "all">): string {
  return categoryKindOptions(category)[0]?.value ?? "support_file";
}

export function resolveCategoryFromPreview(preview: ImportPreviewFileDTO): Exclude<ImportDomainCategory, "all"> {
  return preview.domain_category === "entity" || preview.domain_category === "structured" ? preview.domain_category : "support";
}

export function fileTypeAccent(fileName: string, fileType: string): string {
  const ext = String(fileType || pathTypeFromName(fileName)).toUpperCase();
  if (ext === "PDF") {
    return "is-pdf";
  }
  if (ext === "DOC" || ext === "DOCX") {
    return "is-doc";
  }
  if (ext === "TXT" || ext === "MD") {
    return "is-txt";
  }
  if (["ZIP", "RAR", "7Z", "TAR", "GZ", "TGZ", "BZ2", "TBZ", "XZ", "TXZ"].includes(ext)) {
    return "is-zip";
  }
  if (ext === "XLS" || ext === "XLSX" || ext === "CSV") {
    return "is-sheet";
  }
  return "is-data";
}

export function pathTypeFromName(fileName: string): string {
  const match = /\.([^.]+)$/.exec(String(fileName || "").trim());
  return match?.[1] ?? "";
}

export function resolveFileTypeLabel(fileName: string, fileType: string): string {
  const raw = String(fileType || pathTypeFromName(fileName))
    .replace(/^\./, "")
    .trim()
    .toUpperCase();

  if (!raw) {
    return "FILE";
  }
  if (raw === "JPEG") {
    return "JPG";
  }
  return raw.length > 4 ? raw.slice(0, 4) : raw;
}

export function sanitizeManualFieldMapping(mapping: Record<string, string> | undefined, headers: string[]): Record<string, string> {
  const headerSet = new Set((headers || []).map((header) => String(header || "").trim()).filter(Boolean));
  if (!mapping || headerSet.size === 0) {
    return {};
  }
  return Object.fromEntries(
    Object.entries(mapping).filter(([, header]) => headerSet.has(String(header || "").trim()))
  );
}

export function assignExclusiveManualMapping(mapping: Record<string, string>, fieldKey: string, sourceHeader: string): Record<string, string> {
  const targetKey = String(fieldKey || "").trim();
  const normalizedHeader = String(sourceHeader || "").trim();
  const next = Object.fromEntries(
    Object.entries(mapping).filter(([key, value]) => key !== targetKey && String(value || "").trim() !== normalizedHeader)
  );
  if (targetKey && normalizedHeader) {
    next[targetKey] = normalizedHeader;
  }
  return next;
}

export function clearManualMappingBySource(mapping: Record<string, string>, sourceHeader: string): Record<string, string> {
  const normalizedHeader = String(sourceHeader || "").trim();
  if (!normalizedHeader) {
    return mapping;
  }
  return Object.fromEntries(Object.entries(mapping).filter(([, value]) => String(value || "").trim() !== normalizedHeader));
}

export function sanitizeFieldMappingOrigins(
  origins: Record<string, unknown> | null | undefined,
  mapping: Record<string, string>,
  fallbackOrigin: MappingFieldOrigin = "auto"
): Record<string, MappingFieldOrigin> {
  const validOrigins = new Set<MappingFieldOrigin>(["manual", "auto", "ai", "empty", "info"]);
  const sanitized: Record<string, MappingFieldOrigin> = {};
  for (const key of Object.keys(mapping || {})) {
    const raw = String(origins?.[key] || "").trim().toLowerCase() as MappingFieldOrigin;
    sanitized[key] = validOrigins.has(raw) && raw !== "empty" ? raw : fallbackOrigin;
  }
  return sanitized;
}

export function mappingOriginForMethod(method: string): MappingFieldOrigin {
  return String(method || "").trim().toLowerCase() === "ai_validated" ? "ai" : "auto";
}

export function assignExclusiveFieldMappingWithOrigin(
  mapping: Record<string, string>,
  origins: Record<string, MappingFieldOrigin> | null | undefined,
  fieldKey: string,
  sourceHeader: string,
  origin: MappingFieldOrigin
): { mapping: Record<string, string>; origins: Record<string, MappingFieldOrigin> } {
  const nextMapping = assignExclusiveManualMapping(mapping, fieldKey, sourceHeader);
  const nextOrigins: Record<string, MappingFieldOrigin> = {};
  for (const key of Object.keys(nextMapping)) {
    nextOrigins[key] = key === fieldKey ? origin : origins?.[key] || "auto";
  }
  return { mapping: nextMapping, origins: nextOrigins };
}

export function clearFieldMappingBySourceWithOrigin(
  mapping: Record<string, string>,
  origins: Record<string, MappingFieldOrigin> | null | undefined,
  sourceHeader: string
): { mapping: Record<string, string>; origins: Record<string, MappingFieldOrigin> } {
  const nextMapping = clearManualMappingBySource(mapping, sourceHeader);
  const nextOrigins: Record<string, MappingFieldOrigin> = {};
  for (const key of Object.keys(nextMapping)) {
    nextOrigins[key] = origins?.[key] || "auto";
  }
  return { mapping: nextMapping, origins: nextOrigins };
}

export function shouldAutoExpandMappingDetails(args: {
  archiveChildrenCount: number;
  selectedCategory: Exclude<ImportDomainCategory, "all">;
  selectedKind: string;
  mappingStatus: string;
  mappingMethod: string;
  requiredPendingCount: number;
  invalid: boolean;
}): boolean {
  if (args.archiveChildrenCount > 0 || args.selectedCategory === "support") {
    return false;
  }
  if (!String(args.selectedKind || "").trim() || args.requiredPendingCount > 0 || args.invalid) {
    return true;
  }
  const status = String(args.mappingStatus || "").trim().toLowerCase();
  const method = String(args.mappingMethod || "").trim().toLowerCase();
  if (status === "review") {
    return true;
  }
  return status === "ready" && method !== "" && method !== "exact_header";
}
