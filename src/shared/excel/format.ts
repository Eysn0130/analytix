// Port/adapt of packages/client/ui-sidebar-documentpreview/src/client/excel/format.ts
// DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc; MIT, Copyright (c) 2026 DeepSeek.
// Complete notice: THIRD_PARTY_NOTICES.md.
/** Filename-to-parser selection kept outside the heavy spreadsheet chunk. */
import { ExcelPreviewError } from './error'
import type { ExcelLimits } from './model'

/** Host-owned budgets shared by renderer parsing and the isolated Copy codec. */
export const EXCEL_PREVIEW_LIMITS: Readonly<ExcelLimits> = Object.freeze({
  maxBytes: 16 << 20, maxUncompressedBytes: 64 << 20, maxCells: 250_000, timeoutMs: 15_000
})

/** Maximum complete converted data message; checked before any output allocation. */
export const EXCEL_PREVIEW_MAX_OUTPUT_BYTES = 16 << 20

/** File formats admitted by the spreadsheet preview. */
export type ExcelFormat = 'xlsx' | 'xls' | 'csv' | 'tsv'

/**
 * Resolve a registered filename to its parser.
 * @param path - Decoded file path.
 * @returns The supported suffix; rejects unsupported filenames.
 */
export function excelFormat(path: string): ExcelFormat {
  const suffix = path.slice(path.lastIndexOf('.') + 1).toLowerCase()
  if (suffix === 'xlsx' || suffix === 'xls' || suffix === 'csv' || suffix === 'tsv') return suffix
  throw new ExcelPreviewError('invalid')
}
