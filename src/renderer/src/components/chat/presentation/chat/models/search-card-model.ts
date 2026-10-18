/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ToolBlock } from '../../../../../agent/types'
import { toolResultLocalDisplaySchemaV1, type ToolResultLocalDisplayV1, type ToolResultLocalSearchDataV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
export const CHAT_SEARCH_MAX_LINES = 8
export type SearchCardModel = { search: ToolResultLocalSearchDataV1; recovery: { resultItemId: string; snapshotDigest: string } | undefined }
/** Current Inspect owns input; only historical snapshots use the Search copy. */
export function searchCardInput(display: ToolResultLocalDisplayV1): { pattern: string; source: 'inspect' | 'legacy' } | null {
  const parsed = toolResultLocalDisplaySchemaV1.safeParse(display)
  return parsed.success ? searchInput(parsed.data) : null
}
function searchInput(display: ToolResultLocalDisplayV1): { pattern: string; source: 'inspect' | 'legacy' } | null {
  const name = display.toolName
  if ((name !== 'grep' && name !== 'glob') || display.capture.kind !== 'search') return null
  const input = display.inspect ?? display.capture.search
  if (input.inputTruncated || input.inputJSON === undefined) return null
  let args: unknown
  try { args = JSON.parse(input.inputJSON) } catch { return null }
  if (args === null || typeof args !== 'object' || Array.isArray(args)) return null
  const values = args as Record<string, unknown>
  if (typeof values.pattern !== 'string' || values.pattern.trim() === '' ||
    values.path !== undefined && (typeof values.path !== 'string' || values.path.trim() === '') ||
    name === 'grep' && values.glob !== undefined && typeof values.glob !== 'string') return null
  return { pattern: values.pattern, source: display.inspect ? 'inspect' : 'legacy' }
}
/** Only an authorized, settled root observation supplies structured results. */
export function searchCardModel(block: ToolBlock, display: ToolResultLocalDisplayV1): SearchCardModel | null {
  const name = block.meta?.toolName
  if ((name !== 'grep' && name !== 'glob') || block.toolKind !== 'tool_call' || block.status !== 'success' ||
    block.meta?.child !== undefined || block.meta?.parentCallId !== undefined || ('parentCallId' in block && block.parentCallId !== undefined)) return null
  const parsed = toolResultLocalDisplaySchemaV1.safeParse(display)
  if (!parsed.success || parsed.data.toolName !== name || parsed.data.capture.kind !== 'search') return null
  const capture = parsed.data.capture, search = capture.search
  if (capture.status !== 'completed' || search.toolIsError || searchInput(parsed.data) === null) return null
  const shown = search.shape === 'paths' ? search.paths.length : search.files.reduce((sum, file) => sum + file.matches.length, 0)
  // An oversized first result is not a genuine zero-result search.
  if (shown === 0 && capture.truncated) return null
  return { search, recovery: search.truncated || capture.truncated ? { resultItemId: parsed.data.resultItemId, snapshotDigest: parsed.data.snapshotDigest } : undefined }
}
