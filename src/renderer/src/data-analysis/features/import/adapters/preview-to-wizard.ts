import type { ImportPreviewArchiveChildDTO, ImportPreviewFileDTO } from "../api";
import {
  mappingOriginForMethod,
  resolveCategoryFromPreview,
  sanitizeFieldMappingOrigins,
  sanitizeManualFieldMapping,
  toInt,
  type ImportWizardArchiveChild,
  type ImportWizardFile
} from "../model/import-page-model";

export function sanitizePreviewRows(sampleRows: unknown, headers: string[]): string[][] {
  if (!Array.isArray(sampleRows) || headers.length === 0) {
    return [];
  }
  return sampleRows
    .slice(0, 5)
    .map((row) =>
      headers.map((_, index) => {
        if (!Array.isArray(row)) {
          return "";
        }
        return String(row[index] ?? "");
      })
    )
    .filter((row) => row.some((cell) => String(cell || "").trim()));
}

function fieldMappingsEqual(left: Record<string, string>, right: Record<string, string>): boolean {
  const leftEntries = Object.entries(left || {}).sort(([leftKey], [rightKey]) => leftKey.localeCompare(rightKey));
  const rightEntries = Object.entries(right || {}).sort(([leftKey], [rightKey]) => leftKey.localeCompare(rightKey));
  if (leftEntries.length !== rightEntries.length) {
    return false;
  }
  return leftEntries.every(([key, value], index) => {
    const [rightKey, rightValue] = rightEntries[index] ?? [];
    return key === rightKey && String(value || "").trim() === String(rightValue || "").trim();
  });
}

function resolvePreviewMappingOrigins(args: {
  fieldMapping: Record<string, string>;
  suggestedFieldMapping: Record<string, string>;
  previousFieldMapping: Record<string, string>;
  previousOrigins?: Record<string, unknown>;
  previewOrigins?: Record<string, unknown>;
  mappingMethod: string;
  previousKind?: string;
  previewKind?: string;
}): Record<string, ReturnType<typeof mappingOriginForMethod> | "manual" | "info"> {
  const hasPreviousMapping = Object.keys(args.previousFieldMapping).length > 0;
  const hasPreviousOrigins = Object.keys(args.previousOrigins || {}).length > 0;
  const previousKind = String(args.previousKind || "").trim();
  const previewKind = String(args.previewKind || "").trim();
  const kindMatchesPreview = !previousKind || !previewKind || previousKind === previewKind;
  const matchesSuggestedMapping = kindMatchesPreview && hasPreviousMapping && fieldMappingsEqual(args.previousFieldMapping, args.suggestedFieldMapping);
  const previewFallbackOrigin = mappingOriginForMethod(args.mappingMethod || "");
  const originSeed = hasPreviousMapping
    ? hasPreviousOrigins
      ? args.previousOrigins
      : matchesSuggestedMapping
        ? args.previewOrigins
        : undefined
    : args.previewOrigins;
  const fallbackOrigin = hasPreviousMapping ? (matchesSuggestedMapping ? previewFallbackOrigin : "manual") : previewFallbackOrigin;
  return sanitizeFieldMappingOrigins(originSeed, args.fieldMapping, fallbackOrigin);
}

export function buildWizardArchiveChild(
  preview: ImportPreviewArchiveChildDTO,
  previous?: ImportWizardArchiveChild
): ImportWizardArchiveChild {
  const previewSha256 = String(preview.sha256 || "").trim().toLowerCase();
  const previousSha256 = String(previous?.sha256 || "").trim().toLowerCase();
  const previousForSameReceipt =
    previewSha256 &&
    previousSha256 === previewSha256 &&
    toInt(previous?.size) === toInt(preview.size)
      ? previous
      : undefined;
  const headers = Array.isArray(preview.header_preview) ? preview.header_preview.filter(Boolean) : [];
  const sampleRows = sanitizePreviewRows(preview.sample_rows, headers);
  const suggestedFieldMapping = sanitizeManualFieldMapping(preview.field_mapping, headers);
  const previousFieldMapping = sanitizeManualFieldMapping(previousForSameReceipt?.fieldMapping, headers);
  const hasPreviousMapping = Object.keys(previousFieldMapping).length > 0;
  const fieldMapping = hasPreviousMapping ? previousFieldMapping : suggestedFieldMapping;
  const selectedKind =
    previousForSameReceipt?.selectedKind ||
    preview.suggested_kind ||
    (preview.domain_category === "support" ? "support_file" : "");
  return {
    id: `${preview.archive_path}:${preview.file_name}`,
    fileName: preview.file_name,
    archivePath: preview.archive_path,
    fileType: preview.file_type,
    size: toInt(preview.size),
    rowsTotal: toInt(preview.rows_total),
    columnsTotal: toInt(preview.columns_total),
    headerPreview: headers,
    sampleRows,
    domainCategory:
      preview.domain_category === "entity" || preview.domain_category === "structured" ? preview.domain_category : "support",
    suggestedKind: preview.suggested_kind || "",
    selectedKind,
    suggestedKindLabel: preview.suggested_kind_label || "待映射",
    status: preview.status,
    issue: preview.issue || "",
    detectedBy: preview.detected_by || "",
    sha256: preview.sha256 || "",
    fieldMapping,
    fieldMappingOrigins: resolvePreviewMappingOrigins({
      fieldMapping,
      suggestedFieldMapping,
      previousFieldMapping,
      previousOrigins: previousForSameReceipt?.fieldMappingOrigins,
      previewOrigins: preview.field_mapping_origins,
      mappingMethod: preview.mapping_method || "",
      previousKind: previousForSameReceipt?.selectedKind,
      previewKind: preview.suggested_kind
    }),
    mappingStatus: preview.mapping_status || "",
    mappingMethod: preview.mapping_method || "",
    mappingMessage: preview.mapping_message || "",
    mappingRequiredMissing: Array.isArray(preview.mapping_required_missing) ? preview.mapping_required_missing : []
  };
}

export function buildWizardFile(preview: ImportPreviewFileDTO, previous?: ImportWizardFile): ImportWizardFile {
  const previewSha256 = String(preview.sha256 || "").trim().toLowerCase();
  const previousSha256 = String(previous?.sha256 || "").trim().toLowerCase();
  const previousForSameReceipt =
    previewSha256 &&
    previousSha256 === previewSha256 &&
    toInt(previous?.size) === toInt(preview.size)
      ? previous
      : undefined;
  const category = resolveCategoryFromPreview(preview);
  const headers = Array.isArray(preview.header_preview) ? preview.header_preview.filter(Boolean) : [];
  const sampleRows = sanitizePreviewRows(preview.sample_rows, headers);
  const selectedCategory = previousForSameReceipt?.selectedCategory ?? category;
  const selectedKind =
    previousForSameReceipt?.selectedKind ||
    preview.suggested_kind ||
    (selectedCategory === "support" ? "support_file" : "");
  const requiresPassword = Boolean(preview.requires_password);
  const previousPassword = previousForSameReceipt?.password ?? "";
  const passwordVerified = requiresPassword ? Boolean(previousForSameReceipt?.passwordVerified) : true;
  const suggestedFieldMapping = sanitizeManualFieldMapping(preview.field_mapping, headers);
  const previousFieldMapping = sanitizeManualFieldMapping(previousForSameReceipt?.fieldMapping, headers);
  const hasPreviousMapping = Object.keys(previousFieldMapping).length > 0;
  const fieldMapping = hasPreviousMapping ? previousFieldMapping : suggestedFieldMapping;

  return {
    id: `${preview.source_path}:${preview.file_name}`,
    fileName: preview.file_name,
    sourcePath: preview.source_path,
    fileType: preview.file_type,
    size: toInt(preview.size),
    rowsTotal: toInt(preview.rows_total),
    columnsTotal: toInt(preview.columns_total),
    headerPreview: headers,
    sampleRows,
    domainCategory: category,
    selectedCategory,
    suggestedKind: preview.suggested_kind || "",
    selectedKind,
    suggestedKindLabel: preview.suggested_kind_label || "待映射",
    status: preview.status,
    issue: preview.issue || "",
    acceptsPassword: Boolean(preview.accepts_password),
    requiresPassword,
    password: previousPassword,
    passwordState: requiresPassword ? (passwordVerified ? "verified" : previousForSameReceipt?.passwordState || "idle") : "idle",
    passwordVerified,
    passwordValidationMessage: requiresPassword ? previousForSameReceipt?.passwordValidationMessage || "" : "",
    fieldMapping,
    fieldMappingOrigins: resolvePreviewMappingOrigins({
      fieldMapping,
      suggestedFieldMapping,
      previousFieldMapping,
      previousOrigins: previousForSameReceipt?.fieldMappingOrigins,
      previewOrigins: preview.field_mapping_origins,
      mappingMethod: preview.mapping_method || "",
      previousKind: previousForSameReceipt?.selectedKind,
      previewKind: preview.suggested_kind
    }),
    mappingStatus: preview.mapping_status || "",
    mappingMethod: preview.mapping_method || "",
    mappingMessage: preview.mapping_message || "",
    mappingRequiredMissing: Array.isArray(preview.mapping_required_missing) ? preview.mapping_required_missing : [],
    progress: previousForSameReceipt?.progress ?? 100,
    detectedBy: preview.detected_by || "",
    sha256: preview.sha256 || "",
    archiveChildren: Array.isArray(preview.archive_children)
      ? preview.archive_children.map((child) =>
          buildWizardArchiveChild(
            child,
            previousForSameReceipt?.archiveChildren.find((candidate) => candidate.archivePath === child.archive_path)
          )
        )
      : []
  };
}
