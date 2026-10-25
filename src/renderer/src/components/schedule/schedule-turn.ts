import type { ChatBlock, ToolBlock } from '../../agent/types'
import { conversationCallRecord } from '../chat/presentation/chat/models/conversation-record-model'

export type ScheduleCreationCall = { key: string; block: ToolBlock }
const nativeScheduleOperations = {
  mcp__gui_schedule__gui_schedule_create: 'create', mcp__gui_schedule__claw_schedule_create: 'create',
  mcp__gui_schedule__gui_schedule_update: 'update', mcp__gui_schedule__claw_schedule_update: 'update',
  mcp__gui_schedule__gui_schedule_delete: 'delete', mcp__gui_schedule__claw_schedule_delete: 'delete'
} as const
/** Exact producer names admitted by the private Schedule capture contract. */
export function nativeScheduleOperation(toolName: unknown): 'create' | 'update' | 'delete' | null {
  return typeof toolName === 'string' && Object.hasOwn(nativeScheduleOperations, toolName)
    ? nativeScheduleOperations[toolName as keyof typeof nativeScheduleOperations] : null
}

/** Candidate locators only. A fresh admitted Host receipt still decides whether a card exists.
 * The bound Core primary result cannot be newly appended after terminal CAS. No arrival-order
 * or browser-clock heuristic replaces that authority; late exact replay is deduplicated here. */
export function scheduleCreationCalls(threadId: string | null, turnId: string | null,
  blocks: readonly ChatBlock[], ended: boolean): ScheduleCreationCall[] {
  if (!threadId || !turnId || !ended) return []
  const identities = new Set<string>(), selected: ScheduleCreationCall[] = []
  for (const block of blocks) {
    if (block.kind !== 'tool' || block.toolKind !== 'tool_call' || block.meta?.child || nativeScheduleOperation(block.meta?.toolName) !== 'create' || block.status !== 'success') continue
    const record = conversationCallRecord(threadId, block)
    if (!record || record.turnId !== turnId || !record.toolCallItemId || !record.toolResultItemId || !block.meta?.localResult) continue
    if (identities.has(record.key)) continue
    identities.add(record.key)
    selected.push({ key: record.key, block })
  }
  return selected
}
