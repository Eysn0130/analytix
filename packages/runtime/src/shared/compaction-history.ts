import type { TurnItem } from '../contracts/items.js'

export function effectiveHistoryAfterLatestCompaction(items: readonly TurnItem[]): TurnItem[] {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index]
    if (item.kind === 'compaction' && (item.replacedTokens ?? 0) > 0) {
      return items.slice(index)
    }
  }
  return [...items]
}

export function insertCompactionIntoVisibleHistory(input: {
  visibleItems: readonly TurnItem[]
  compactedItems: readonly TurnItem[]
  summaryItem: TurnItem
}): TurnItem[] {
  const summaryIndex = input.compactedItems.findIndex((item) => item.id === input.summaryItem.id)
  if (summaryIndex < 0) return replaceOrAppendItem(input.visibleItems, input.summaryItem)

  const tailIds = new Set(
    input.compactedItems
      .slice(summaryIndex + 1)
      .map((item) => item.id)
  )
  const withoutSummary = input.visibleItems.filter((item) => item.id !== input.summaryItem.id)
  if (tailIds.size === 0) return [...withoutSummary, input.summaryItem]

  const insertIndex = withoutSummary.findIndex((item) => tailIds.has(item.id))
  if (insertIndex < 0) return [...withoutSummary, input.summaryItem]

  return [
    ...withoutSummary.slice(0, insertIndex),
    input.summaryItem,
    ...withoutSummary.slice(insertIndex)
  ]
}

function replaceOrAppendItem(items: readonly TurnItem[], item: TurnItem): TurnItem[] {
  const index = items.findIndex((existing) => existing.id === item.id)
  if (index < 0) return [...items, item]
  return items.map((existing) => (existing.id === item.id ? item : existing))
}

/**
 * Reorder a turn's items so any compaction summary lands last in the bucket.
 *
 * The session-store layout keeps `[head, summary, tail]` so the model receives
 * `[summary, tail]` after compaction. The thread-store layout drives the
 * renderer's grouped timeline; moving summaries to the end of the relevant
 * turn keeps the visible progress row attached to the turn that triggered it.
 */
export function placeCompactionsAtTurnEnd(items: readonly TurnItem[]): TurnItem[] {
  let hasTrailingCompaction = false
  for (const item of items) {
    if (item.kind === 'compaction' && (item.replacedTokens ?? 0) > 0) {
      hasTrailingCompaction = true
      break
    }
  }
  if (!hasTrailingCompaction) return [...items]
  const rest: TurnItem[] = []
  const trailing: TurnItem[] = []
  for (const item of items) {
    if (item.kind === 'compaction' && (item.replacedTokens ?? 0) > 0) {
      trailing.push(item)
    } else {
      rest.push(item)
    }
  }
  return [...rest, ...trailing]
}
