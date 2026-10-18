// Port/adapt of packages/client/ui-sidebar-documentpreview/src/client/excel/worker.ts
// DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc; MIT, Copyright (c) 2026 DeepSeek.
// Complete notice: THIRD_PARTY_NOTICES.md.
/** One workbook per disposable parser Worker; formulas and external links are never executed. */
import { convertExcel } from '../../../../../shared/excel/convert'
import type { ExcelFormat } from '../../../../../shared/excel/format'
import { ExcelPreviewError } from '../../../../../shared/excel/error'
import type { ExcelLimits } from '../../../../../shared/excel/model'

globalThis.onmessage = (event: MessageEvent<{ bytes: Uint8Array<ArrayBuffer>; format: ExcelFormat; limits: ExcelLimits }>) => {
  void convertExcel(event.data.bytes, event.data.format, event.data.limits).then(
    (value) => { globalThis.postMessage({ ok: true, value }) },
    (error: unknown) => { globalThis.postMessage({ ok: false, code: error instanceof ExcelPreviewError ? error.code : 'invalid' }) },
  )
}
