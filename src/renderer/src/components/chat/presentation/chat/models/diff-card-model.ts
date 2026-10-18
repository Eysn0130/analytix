/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
export const CHAT_DIFF_MAX_LINES = 9
export type DiffRow = { kind: 'path' | 'del' | 'add' | 'context' | 'gap'; text: string }
export type DiffCardModel = { rows: readonly DiffRow[]; path?: string; totals: { added: number; removed: number }; approximate: boolean }
/** Parse only an already authorized Host patch, never tool prose or intended arguments. */
export function diffCardModel(block: ProcessDisplayBlockV1): DiffCardModel | null {
  if (!['write', 'edit'].includes(block.kind) || !['completed', 'failed', 'timeout', 'canceled'].includes(block.state) || block.effectApplied !== true ||
    !block.body || block.classification === 'withheld') return null
  const body = block.body
  const rows: DiffRow[] = []
  let inHunk = false
  for (const line of body.text.split('\n')) {
    if (!inHunk && (line.startsWith('--- ') || line.startsWith('+++ ') || line.startsWith('diff --git ') || line.startsWith('index '))) continue
    if (/^@@ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@/.test(line)) { if (inHunk) rows.push({ kind: 'gap', text: '⋯' }); inHunk = true; continue }
    if (!inHunk) { if (line !== '') return null; continue }
    if (line.startsWith('+')) rows.push({ kind: 'add', text: line.slice(1) })
    else if (line.startsWith('-')) rows.push({ kind: 'del', text: line.slice(1) })
    else if (line.startsWith(' ')) rows.push({ kind: 'context', text: line.slice(1) })
    else if (line !== '' && line !== '\\ No newline at end of file') return null
  }
  // An identical overwrite can have no patch. Do not invent a replacement diff.
  if (!rows.length) return null
  if (block.path) rows.unshift({ kind: 'path', text: block.path })
  return { rows, path: block.path, totals: { added: body.addedLines, removed: body.removedLines }, approximate: body.approximate }
}
