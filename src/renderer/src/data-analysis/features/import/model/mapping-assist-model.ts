import type { MappingFieldOrigin } from "./import-page-model";

export function applySuggestedMappingEntry(
  mapping: Record<string, string>,
  origins: Record<string, MappingFieldOrigin>,
  fieldKey: string,
  sourceHeader: string,
  origin: MappingFieldOrigin
): boolean {
  const normalizedHeader = String(sourceHeader || "").trim();
  if (!fieldKey || !normalizedHeader) {
    return false;
  }
  const previousHeader = String(mapping[fieldKey] || "").trim();
  const previousOrigin = origins[fieldKey];
  for (const key of Object.keys(mapping)) {
    if (key !== fieldKey && String(mapping[key] || "").trim() === normalizedHeader) {
      delete mapping[key];
      delete origins[key];
    }
  }
  mapping[fieldKey] = normalizedHeader;
  origins[fieldKey] = origin;
  return previousHeader !== normalizedHeader || previousOrigin !== origin;
}

export function applySuggestedMappingEntries(
  mapping: Record<string, string>,
  origins: Record<string, MappingFieldOrigin>,
  suggestions: Record<string, string>,
  originForField: (fieldKey: string) => MappingFieldOrigin
): number {
  let appliedCount = 0;
  for (const [fieldKey, sourceHeader] of Object.entries(suggestions)) {
    if (applySuggestedMappingEntry(mapping, origins, fieldKey, sourceHeader, originForField(fieldKey))) {
      appliedCount += 1;
    }
  }
  return appliedCount;
}
