// Port/adapt of packages/client/ui-sidebar-documentpreview/src/client/excel/error.ts
// DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc; MIT, Copyright (c) 2026 DeepSeek.
// Complete notice: THIRD_PARTY_NOTICES.md.
/** Locale-independent spreadsheet parser failure categories. */
export class ExcelPreviewError extends Error {
  /** @param code - User-actionable parser failure. */
  constructor(readonly code: 'invalid' | 'tooLarge' | 'timeout' | 'encoding', options?: ErrorOptions) {
    super(code, options)
    this.name = 'ExcelPreviewError'
  }
}
