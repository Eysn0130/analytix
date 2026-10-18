/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ReadBlockProps } from '../../ReadBlock'
export const CHAT_READ_MAX_LINES = 8
export type ReadCardModel = Pick<ReadBlockProps, 'label' | 'lines' | 'totalLines' | 'lang'> & { truncated: boolean; filePathLine?: number }
export function languageForPath(path: string): string | undefined {
  const ext = path.match(/\.([A-Za-z0-9]+)$/)?.[1]?.toLowerCase()
  return ext ? ({ tsx: 'tsx', jsx: 'jsx', sh: 'bash', yml: 'yaml', md: 'markdown' } as Record<string, string>)[ext] ?? ext : undefined
}
/** Existing authorized capture, including children, supplies actual file coordinates. */
export function readCardModel(display: ToolResultLocalDisplayV1): ReadCardModel | null {
  const capture = display.capture
  if (capture.kind !== 'read' || capture.status !== 'completed') return null
  const texts = capture.body === '' ? [] : capture.body.split(/\r\n|\r|\n/)
  if (texts.at(-1) === '') texts.pop()
  if (texts.length && (!Number.isSafeInteger(capture.startLine) || capture.startLine < 1)) return null
  const lines = texts.map((text, index) => ({ number: capture.startLine + index, text }))
  if (lines.some(line => line.number > capture.endLine || line.number > capture.totalLines)) return null
  return { label: capture.label, lines, totalLines: capture.totalLines, lang: languageForPath(capture.label),
    truncated: capture.truncated, filePathLine: capture.startLine > 0 ? capture.startLine : undefined }
}
