/* Adapted from pinned DSH turn-deliverables, 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import type { ChatBlock, ToolBlock } from '../../../../agent/types'
import { toolResultLocalPresentHistorySchemaV1, toolResultLocalSelectorSchemaV1,
  type ToolResultLocalPresentHistoryV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'

export type PresentCall = { key: string; block: ToolBlock; history?: ToolResultLocalPresentHistoryV1 }
/** Only the private primary-order witness supplies closing and result positions. */
export function presentCallsForClosing(threadId: string | null, turnId: string | null, blocks: readonly ChatBlock[],
  history: ToolResultLocalPresentHistoryV1 | null, closing: ChatBlock | null, ended: boolean): PresentCall[] {
  const parsed = toolResultLocalPresentHistorySchemaV1.safeParse(history)
  if (!threadId || !turnId || !parsed.success || parsed.data.threadId !== threadId || parsed.data.turnId !== turnId) return []
  const ordered = parsed.data.orderedItems
  const closingIndex = closing?.kind === 'assistant' && closing.text.trim() && closing.meta?.turnId === turnId
    ? ordered.findIndex(item => item.kind === 'assistant_text' && item.itemId === closing.id) : -1
  if (closing ? closingIndex < 0 : !ended || ordered.some(item => item.kind === 'assistant_text')) return []
  return ordered.slice(0, closing ? closingIndex : ordered.length).flatMap(item => {
    if (item.kind !== 'tool_result') return []
    const identity = { threadId, turnId, callId: item.callId, resultItemId: item.itemId }
    const original = blocks.find((block): block is ToolBlock => {
      if (block.kind !== 'tool' || block.toolKind !== 'tool_call' || block.status === 'running' || block.meta?.toolName !== 'present') return false
      const result = toolResultLocalSelectorSchemaV1.safeParse({ ...block.meta.localResult, threadId })
      return result.success && JSON.stringify(result.data) === JSON.stringify(identity) &&
        block.meta.turnId === turnId && block.meta.callId === item.callId
    })
    // Case public history intentionally has no tool body or call seat. This
    // private anchor only opens the original paired snapshot; it is not stored
    // or projected as a transcript event, nor does it assert success itself.
    const block: ToolBlock = original ?? { kind: 'tool', id: 'present-history:' + item.itemId,
      summary: 'present', status: 'success', toolKind: 'tool_call', meta: {
        toolName: 'present', turnId, callId: item.callId, recordThreadId: threadId, toolResultItemId: item.itemId,
        toolResultIdentity: { threadId, turnId, callId: item.callId, itemId: item.itemId },
        localResult: { turnId, callId: item.callId, resultItemId: item.itemId }
      } }
    return [{ key: JSON.stringify(identity), block, history: history! }]
  })
}
export function presentedBasename(path: string): string { return path.split(/[\\/]/).filter(Boolean).at(-1) ?? path }
export function presentedDescription(description: string | undefined, path: string): string {
  const clean = description?.replace(/\s*(?:\([^()]*\)|（[^（）]*）)\s*$/u, '').trim()
  const name = presentedBasename(path), dot = name.lastIndexOf('.')
  return clean || (dot > 0 ? name.slice(dot + 1).toUpperCase() : 'FILE')
}
/** Native IDs decide equality; set preserves the first position and the last declared value. */
export function latestPresentedFiles<T extends { key: string; fileIdentity?: string }>(files: readonly T[]): T[] {
  const latest = new Map<string, T>()
  for (const file of files) latest.set(file.fileIdentity ?? file.key, file)
  return [...latest.values()]
}
