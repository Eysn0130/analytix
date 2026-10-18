/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ChatBlock, ToolBlock } from '../../../../../agent/types'
import type { ThreadTurn } from '../../../../../thread/projection/thread-turns'

/** Renderer-local identities, never a raw trajectory or disclosure grant. */
export type ConversationCallIdentity = {
  threadId: string; turnId: string; callId: string
  toolCallItemId?: string; toolResultItemId?: string
}
export type ConversationInspectRequest = ConversationCallIdentity & { version: number }
export type ConversationCallRecord = ConversationCallIdentity & { key: string; block: ToolBlock }
const identity = (value: unknown): value is string => {
  if (typeof value !== 'string' || value.length === 0 || value.length > 512) return false
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index)
    if (code < 0x20 || code === 0x7f) return false
  }
  return true
}

export function conversationCallRecord(threadId: string, block: ChatBlock): ConversationCallRecord | null {
  if (block.kind !== 'tool' || block.meta?.recordThreadId !== threadId || !identity(threadId) ||
    !identity(block.meta.turnId) || !identity(block.meta.callId)) return null
  const { toolCallItemId, toolResultItemId } = block.meta
  if ((!identity(toolCallItemId) && !identity(toolResultItemId)) ||
    toolCallItemId !== undefined && !identity(toolCallItemId) || toolResultItemId !== undefined && !identity(toolResultItemId)) return null
  const matches = (value: unknown, itemId: string | undefined) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return false
    const source = value as Record<string, unknown>
    return source.threadId === threadId && source.turnId === block.meta?.turnId && source.callId === block.meta?.callId && source.itemId === itemId
  }
  if (toolCallItemId && !matches(block.meta.toolCallIdentity, toolCallItemId) ||
    toolResultItemId && !matches(block.meta.toolResultIdentity, toolResultItemId)) return null
  const local = block.meta.localResult
  if (local && (local.turnId !== block.meta.turnId || local.callId !== block.meta.callId || local.resultItemId !== toolResultItemId)) return null
  return { key: JSON.stringify([threadId, block.meta.turnId, block.meta.callId, toolCallItemId ?? null, toolResultItemId ?? null]), threadId,
    turnId: block.meta.turnId, callId: block.meta.callId,
    ...(toolCallItemId ? { toolCallItemId } : {}), ...(toolResultItemId ? { toolResultItemId } : {}), block }
}

/** Preserve admitted load order. A duplicate identity remains ambiguous. */
export function conversationCallRecords(threadId: string | null, blocks: readonly ChatBlock[]): ConversationCallRecord[] {
  return threadId ? blocks.flatMap(block => { const record = conversationCallRecord(threadId, block); return record ? [record] : [] }) : []
}
/** Records use admitted native turn IDs, never the nearest public user/assistant. */
export function conversationRecordTurns(threadId: string | null, blocks: readonly ChatBlock[]): ThreadTurn[] {
  if (!threadId) return []
  const turns = new Map<string, ThreadTurn>()
  for (const block of blocks) {
    const turnId = block.kind === 'tool' ? conversationCallRecord(threadId, block)?.turnId :
      block.kind === 'user' || block.kind === 'assistant' || block.kind === 'compaction' ? block.meta?.turnId : undefined
    if (!identity(turnId)) continue
    let turn = turns.get(turnId)
    if (!turn) { turn = { blocks: [] }; turns.set(turnId, turn) }
    if (block.kind === 'user' && !turn.user) turn.user = block
    else turn.blocks.push(block)
  }
  return [...turns.values()]
}
export function locateConversationCall(records: readonly ConversationCallRecord[], request: ConversationCallIdentity):
  { kind: 'found'; record: ConversationCallRecord } | { kind: 'missing' | 'ambiguous' } {
  const matches = records.filter(record => record.threadId === request.threadId && record.turnId === request.turnId && record.callId === request.callId &&
    (!request.toolCallItemId || record.toolCallItemId === request.toolCallItemId) &&
    (!request.toolResultItemId || record.toolResultItemId === request.toolResultItemId))
  return matches.length === 1 ? { kind: 'found', record: matches[0] } : { kind: matches.length ? 'ambiguous' : 'missing' }
}
