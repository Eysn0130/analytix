import type { ThreadProjectionSnapshot, ThreadRow } from './thread-row-model'

export function selectThreadRows(snapshot: ThreadProjectionSnapshot): ThreadRow[] {
  return snapshot.rows
}

export function selectVisibleTurnRows(snapshot: ThreadProjectionSnapshot): Extract<ThreadRow, { kind: 'turn' }>[] {
  return snapshot.rows.filter((row): row is Extract<ThreadRow, { kind: 'turn' }> => row.kind === 'turn')
}

export function selectThreadHasStreamingRow(snapshot: ThreadProjectionSnapshot): boolean {
  return snapshot.rows.some((row) => {
    if (row.kind === 'turn') return row.turn.hasLiveStream || row.turn.isProcessing
    if (row.kind === 'liveOnlyTurn') {
      return row.live.trim().length > 0 || Boolean(row.turnId)
    }
    return false
  })
}
