// Port/adapt of packages/client/ui-sidebar-documentpreview/src/client/excel/parse.ts
// DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc; MIT, Copyright (c) 2026 DeepSeek.
// Complete notice: THIRD_PARTY_NOTICES.md.
/** Browser lifetime for the isolated spreadsheet parser. */
import type { ExcelFormat } from '../../../../../shared/excel/format'
import { EXCEL_UNSUPPORTED_FEATURES, type ExcelLimits, type ExcelPreview } from '../../../../../shared/excel/model'

/**
 * Parse workbook bytes in a disposable Worker, copying the retained preview buffer.
 * @param bytes - Borrowed complete file bytes.
 * @param format - Format selected by the registered filename suffix.
 * @param limits - File, matrix, and elapsed-time limits.
 * @param signal - Document body's lifetime.
 * @returns Parsed sheets; rejects with a locale key on parser failures.
 */
export function parseExcel(
  bytes: Uint8Array<ArrayBuffer>, format: ExcelFormat, limits: ExcelLimits, signal: AbortSignal,
): Promise<ExcelPreview> {
  signal.throwIfAborted()
  if (bytes.byteLength > limits.maxBytes) return Promise.reject(new Error('tooLarge'))
  return new Promise((resolve, reject) => {
    let worker: Worker
    try {
      worker = new Worker(new URL('./worker.ts', import.meta.url), { type: 'module', name: 'analytix-excel-preview' })
    } catch (error) {
      reject(new Error('invalid', { cause: error }))
      return
    }
    let settled = false
    const finish = (): boolean => {
      if (settled) return false
      settled = true
      clearTimeout(timer)
      signal.removeEventListener('abort', abort)
      worker.onmessage = null
      worker.onerror = null
      worker.onmessageerror = null
      worker.terminate()
      return true
    }
    const abort = (): void => { if (finish()) reject(new DOMException('Excel preview closed', 'AbortError')) }
    const fail = (): void => { if (finish()) reject(new Error('invalid')) }
    const timer = setTimeout(() => { if (finish()) reject(new Error('timeout')) }, limits.timeoutMs)
    worker.onerror = fail
    worker.onmessageerror = fail
    worker.onmessage = (event: MessageEvent<unknown>) => {
      const message = event.data
      if (typeof message !== 'object' || message === null || !('ok' in message)) { fail(); return }
      if (message.ok === true && 'value' in message && validPreview(message.value, limits)) {
        if (finish()) resolve(message.value)
      } else if (message.ok === false && 'code' in message && ['invalid', 'tooLarge', 'timeout', 'encoding'].includes(String(message.code))) {
        if (finish()) reject(new Error(String(message.code)))
      } else fail()
    }
    signal.addEventListener('abort', abort, { once: true })
    try {
      const copy = bytes.slice()
      worker.postMessage({ bytes: copy, format, limits }, [copy.buffer])
    } catch (error) { if (finish()) reject(new Error('invalid', { cause: error })) }
  })
}

function validPreview(value: unknown, limits: ExcelLimits): value is ExcelPreview {
  if (typeof value !== 'object' || value === null || !('sheets' in value) || !('missingResults' in value)) return false
  if (typeof value.missingResults !== 'number' || !Number.isSafeInteger(value.missingResults) || value.missingResults < 0 ||
      !('unsupportedFeatures' in value) || !Array.isArray(value.unsupportedFeatures) ||
      !value.unsupportedFeatures.every((feature: unknown) => EXCEL_UNSUPPORTED_FEATURES.some(known => known === feature)) ||
      !Array.isArray(value.sheets) || value.sheets.length === 0 || value.sheets.length > limits.maxCells) return false
  const integer = (n: unknown, min: number, max: number): n is number => typeof n === 'number' && Number.isSafeInteger(n) && n >= min && n <= max
  let area = 0, visible = false
  const ids = new Set<string>()
  for (const sheet of value.sheets as unknown[]) {
    if (typeof sheet !== 'object' || sheet === null || !('id' in sheet) || typeof sheet.id !== 'string' ||
        !sheet.id || sheet.id.length > 4096 || ids.has(sheet.id) || !('name' in sheet) || typeof sheet.name !== 'string' ||
        !('row' in sheet) || !integer(sheet.row, 1, limits.maxCells) || !('column' in sheet) || !integer(sheet.column, 1, limits.maxCells) ||
        !('hide' in sheet) || sheet.hide !== 0 && sheet.hide !== 1 || !('celldata' in sheet) || !Array.isArray(sheet.celldata)) return false
    ids.add(sheet.id); visible ||= sheet.hide === 0
    area += sheet.row * sheet.column
    if (!Number.isSafeInteger(area) || area > limits.maxCells || sheet.celldata.length > sheet.row * sheet.column) return false
    for (const cell of sheet.celldata as unknown[]) {
      if (typeof cell !== 'object' || cell === null || !('r' in cell) || !integer(cell.r, 0, sheet.row - 1) ||
          !('c' in cell) || !integer(cell.c, 0, sheet.column - 1) || !('v' in cell) || typeof cell.v !== 'object' || cell.v === null) return false
    }
  }
  return visible && value.missingResults <= limits.maxCells
}
