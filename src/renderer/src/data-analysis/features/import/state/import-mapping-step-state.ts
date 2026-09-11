export interface ImportMappingStepItemState {
  invalid?: boolean;
  requiredMissingCount?: number;
}

export function isImportMappingStepItemBlocking(item: ImportMappingStepItemState): boolean {
  return Boolean(item.invalid) || Math.max(0, Number(item.requiredMissingCount || 0)) > 0;
}

export function canContinueImportMappingStep(items: ImportMappingStepItemState[]): boolean {
  return items.length > 0 && !items.some(isImportMappingStepItemBlocking);
}
