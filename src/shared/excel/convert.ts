// Port/adapt of packages/client/ui-sidebar-documentpreview/src/client/excel/convert.ts
// DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc; MIT, Copyright (c) 2026 DeepSeek.
// Complete notice: THIRD_PARTY_NOTICES.md.
/** Format-specific parsing into one read-only FortuneSheet representation. */
import { EXCEL_PREVIEW_MAX_OUTPUT_BYTES, type ExcelFormat } from './format'
import { ExcelPreviewError } from './error'
import type { ExcelLimits, ExcelPreview } from './model'
import { convertXlsx } from './xlsx'
import { convertXls } from './xls'
import { convertDelimited } from './delimited'

/**
 * Decode spreadsheet bytes without recalculating saved formulas.
 * @param bytes - Borrowed complete file bytes.
 * @param format - Format selected by the registered filename suffix.
 * @param limits - File and matrix allocation limits.
 * @returns Display sheets and missing formula-cache count.
 */
export async function convertExcel(bytes: Uint8Array<ArrayBuffer>, format: ExcelFormat, limits: ExcelLimits): Promise<ExcelPreview> {
  if (bytes.byteLength > limits.maxBytes) throw new ExcelPreviewError('tooLarge')
  const preview = format === 'xlsx' ? await convertXlsx(bytes, limits)
    : format === 'xls' ? convertXls(bytes, limits) : convertDelimited(bytes, format, limits)
  // Repeated shared strings can amplify a bounded ZIP across many cells.
  // Bound the complete data message before structured-clone or JSON allocation.
  assertOutputBudget(preview)
  return preview
}


function assertOutputBudget(value: unknown): void {
  let bytes = 0
  const add = (count: number): void => {
    bytes += count
    if (bytes > EXCEL_PREVIEW_MAX_OUTPUT_BYTES) throw new ExcelPreviewError('tooLarge')
  }
  const string = (text: string): void => {
    add(2)
    for (let index = 0; index < text.length; index++) {
      const code = text.charCodeAt(index)
      if (code >= 0xd800 && code <= 0xdbff && index + 1 < text.length && text.charCodeAt(index + 1) >= 0xdc00 && text.charCodeAt(index + 1) <= 0xdfff) { add(4); index++ }
      else if ([8, 9, 10, 12, 13].includes(code)) add(2)
      else if (code < 32 || code >= 0xd800 && code <= 0xdfff) add(6)
      else if (code === 34 || code === 92) add(2)
      else add(code < 128 ? 1 : code < 2048 ? 2 : 3)
    }
  }
  const visit = (item: unknown): void => {
    if (item === null) { add(4); return }
    if (typeof item === 'string') { string(item); return }
    if (typeof item === 'boolean') { add(item ? 4 : 5); return }
    if (typeof item === 'number') {
      if (!Number.isFinite(item)) throw new ExcelPreviewError('invalid')
      add(String(item).length); return
    }
    if (Array.isArray(item)) {
      add(2)
      for (let index = 0; index < item.length; index++) { if (index) add(1); visit(item[index] ?? null) }
      return
    }
    if (typeof item !== 'object') throw new ExcelPreviewError('invalid')
    add(2)
    let count = 0
    for (const [key, entry] of Object.entries(item)) {
      if (entry === undefined) continue
      if (count++) add(1)
      string(key); add(1); visit(entry)
    }
  }
  visit(value)
}
