import type { TurnItem } from '../contracts/items.js'
import type { ModelHistoryItem } from '../ports/model-client.js'
import { makePublicToolResultWithheldProjection } from './item.js'

export const INTERRUPTED_TOOL_RESULT_PLACEHOLDER =
  '[no result: the previous turn was interrupted before this tool call completed]'

/**
 * Repairs persisted turn items into a model-sendable history shape.
 *
 * Analytix stores GUI-only items such as approvals, user input prompts, and
 * reasoning blocks beside model-bound tool calls. Provider APIs are stricter:
 * every assistant tool-call block must be followed by exactly one matching
 * result per call, with only model-ignored bridge items in between.
 */
export function repairModelHistoryItems(items: TurnItem[]): TurnItem[]
export function repairModelHistoryItems(items: ModelHistoryItem[]): ModelHistoryItem[]
export function repairModelHistoryItems(items: ModelHistoryItem[]): ModelHistoryItem[] {
  const repaired: ModelHistoryItem[] = []
  let changed = false
  let index = 0
  while (index < items.length) {
    const item = items[index]
    if (item?.kind === 'tool_result') {
      changed = true
      index += 1
      continue
    }
    if (item?.kind !== 'tool_call') {
      repaired.push(item)
      index += 1
      continue
    }

    const calls: Array<{ item: Extract<ModelHistoryItem, { kind: 'tool_call' }>; index: number }> = []
    const seenCallIds = new Set<string>()
    let cursor = index
    while (cursor < items.length && items[cursor]?.kind === 'tool_call') {
      const call = items[cursor] as Extract<ModelHistoryItem, { kind: 'tool_call' }>
      if (!seenCallIds.has(call.callId)) {
        seenCallIds.add(call.callId)
        calls.push({ item: call, index: cursor })
      } else {
        changed = true
      }
      cursor += 1
    }

    const result = findResultBlock(items, cursor, {
      turnId: item.turnId,
      expectedCallIds: seenCallIds
    })
    for (const call of calls) repaired.push(call.item)
    for (const bridgeItem of result.bridgeItems) repaired.push(bridgeItem)
    for (const resultIndex of result.resultIndexes) repaired.push(items[resultIndex])
    for (const call of calls) {
      if (result.resultCallIds.has(call.item.callId)) continue
      changed = true
      repaired.push(makeInterruptedToolResult(call.item))
    }

    if (result.changed) {
      changed = true
    }
    index = result.nextIndex
  }
  return changed ? repaired : items
}

function findResultBlock(
  items: ModelHistoryItem[],
  startIndex: number,
  options: { turnId: string; expectedCallIds: Set<string> }
): {
  resultCallIds: Set<string>
  resultIndexes: number[]
  bridgeItems: ModelHistoryItem[]
  changed: boolean
  nextIndex: number
} {
  const seenResultIds = new Set<string>()
  const resultIndexes: number[] = []
  const bridgeItems: ModelHistoryItem[] = []
  let changed = false
  let sawResult = false
  let index = startIndex

  while (index < items.length) {
    const item = items[index]
    if (!item) break
    if (item.kind === 'tool_result') {
      sawResult = true
      if (options.expectedCallIds.has(item.callId) && !seenResultIds.has(item.callId)) {
        seenResultIds.add(item.callId)
        resultIndexes.push(index)
      } else {
        changed = true
      }
      index += 1
      continue
    }
    if (isToolResultBridgeItem(item, { turnId: options.turnId, sawResult })) {
      bridgeItems.push(item)
      index += 1
      continue
    }
    break
  }

  return { resultCallIds: seenResultIds, resultIndexes, bridgeItems, changed, nextIndex: index }
}

function makeInterruptedToolResult(call: Extract<ModelHistoryItem, { kind: 'tool_call' }>): Extract<TurnItem, { kind: 'tool_result' }> {
  const now = new Date().toISOString()
  return {
    id: `tool_result_repaired_${call.callId}`,
    turnId: call.turnId,
    threadId: call.threadId,
    role: 'tool',
    status: 'aborted',
    createdAt: now,
    finishedAt: now,
    kind: 'tool_result',
    toolName: call.toolName,
    callId: call.callId,
    toolKind: call.toolKind,
    output: makePublicToolResultWithheldProjection({ status: 'cancelled' }),
    isError: true
  }
}

export function isToolResultBridgeItem(
  item: ModelHistoryItem,
  options: { turnId: string; sawResult: boolean }
): boolean {
  switch (item.kind) {
    case 'assistant_reasoning':
    case 'approval':
    case 'user_input':
    case 'error':
      return true
    case 'assistant_text':
      return !options.sawResult && item.turnId === options.turnId
    default:
      return false
  }
}
