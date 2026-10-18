import { TrajectoryProjection, type TrajectorySnapshot } from './trajectory-adapter'
import { isStrictPublicRuntimeSseIpcPayload } from '@shared/public-runtime-sse'
import { isPublicProjectionRevoked } from '../../lib/public-projection-revocation'

export type TrajectoryHistoryReader = (throughSeq: number, accept: (events: readonly unknown[]) => void, signal: AbortSignal) => Promise<void>
type Entry = {
  projection: TrajectoryProjection; leases: number; listeners: Set<() => void>; snapshot: TrajectorySnapshot
  reader?: TrajectoryHistoryReader; older?: TrajectoryProjection; request?: AbortController; requestThroughSeq?: number
  olderHistory: NonNullable<TrajectorySnapshot['olderHistory']>; knownThroughSeq: number
}
const EMPTY: TrajectorySnapshot = { threadId: '', records: [], loadedFromSeq: null, loadedThroughSeq: null,
  completeThroughSeq: null, truncated: false, discardedThroughSeq: null, history: 'unknown' }

/** One current display binding plus actual view/replay/stream leases; no transport or LRU. */
export class TrajectorySource {
  private entries = new Map<string, Entry>()
  private currentThread: string | null = null
  private entry(threadId: string): Entry {
    let entry = this.entries.get(threadId)
    if (!entry) {
      const projection = new TrajectoryProjection(threadId)
      entry = { projection, snapshot: projection.getSnapshot(), leases: 0, listeners: new Set(), olderHistory: 'unavailable', knownThroughSeq: 0 }
      if (isPublicProjectionRevoked(threadId)) { entry.projection.revoke(); entry.snapshot = entry.projection.getSnapshot() }
      this.entries.set(threadId, entry)
    }
    return entry
  }
  private retire(threadId: string, entry: Entry): void {
    if (this.currentThread !== threadId && !entry.leases && !entry.listeners.size && this.entries.get(threadId) === entry) this.entries.delete(threadId)
  }
  bindThread(threadId: string): void {
    const previous = this.currentThread
    this.entry(threadId); this.currentThread = threadId
    if (previous && previous !== threadId) {
      const entry = this.entries.get(previous)
      if (entry) { this.cancelOlder(entry); entry.older = undefined; this.notify(previous); this.retire(previous, entry) }
    }
  }
  retainThread(threadId: string): () => void {
    const entry = this.entry(threadId)
    entry.leases++
    let released = false
    return () => { if (released) return; released = true; entry.leases--; this.retire(threadId, entry) }
  }
  getSnapshot = (threadId: string): TrajectorySnapshot => this.entries.get(threadId)?.snapshot ?? EMPTY
  subscribe = (threadId: string, listener: () => void): (() => void) => {
    const entry = this.entry(threadId)
    entry.listeners.add(listener)
    return () => {
      entry.listeners.delete(listener)
      if (!entry.listeners.size) { this.cancelOlder(entry); entry.older = undefined; this.notify(threadId) }
      this.retire(threadId, entry)
    }
  }
  private notify(threadId: string): void {
    const entry = this.entries.get(threadId)
    if (entry) entry.snapshot = { ...(entry.older ?? entry.projection).getSnapshot(),
      historyWindow: entry.older ? 'older' : 'latest', olderHistory: entry.olderHistory,
      canLoadOlder: entry.olderHistory !== 'exhausted' && this.canReadOlder(entry) }
    for (const listener of this.entries.get(threadId)?.listeners ?? []) {
      try { listener() } catch { /* Observers cannot affect accepted dispatch or ACK. */ }
    }
  }
  observe(threadId: string, events: readonly unknown[]): void {
    // A late old-stream delivery cannot recreate a retired thread cache.
    try {
      const entry = this.entries.get(threadId)
      if (!entry?.projection.accept(events)) return
      if (entry.projection.getSnapshot().history === 'revoked') { this.revoke(threadId); return }
      // The detached replay cannot miss a newer finish/update while it is in flight.
      // Invalidate instead of buffering another event log or publishing stale state.
      if (events.some(event => (event as { kind?: string }).kind === 'thread_rewound' ||
          entry.requestThroughSeq !== undefined && (event as { seq?: number }).seq! > entry.requestThroughSeq)) this.cancelOlder(entry)
      entry.older?.accept(events)
      if (!entry.request && entry.olderHistory !== 'exhausted') entry.olderHistory = this.canReadOlder(entry) ? 'available' : 'unavailable'
      this.notify(threadId)
    } catch { /* Display-only observer. */ }
  }
  revoke(threadId: string): void {
    const entry = this.entries.get(threadId)
    if (entry) { this.cancelOlder(entry); entry.older = undefined; entry.reader = undefined; entry.olderHistory = 'unavailable'; entry.projection.revoke() }
    this.notify(threadId)
  }
  private cancelOlder(entry: Entry): void {
    const request = entry.request
    entry.request = undefined; entry.requestThroughSeq = undefined
    request?.abort()
    if (entry.olderHistory === 'loading') entry.olderHistory = this.canReadOlder(entry) ? 'available' : 'unavailable'
  }
  private olderBoundary(entry: Entry): number | null {
    const snapshot = (entry.older ?? entry.projection).getSnapshot()
    return snapshot.discardedThroughSeq !== null ? snapshot.discardedThroughSeq + 1
      : snapshot.completeThroughSeq === null ? snapshot.records[0]?.firstSeq ?? null : null
  }
  private canReadOlder(entry: Entry): boolean {
    const before = this.olderBoundary(entry)
    return Boolean(entry.reader && entry.projection.getSnapshot().history !== 'revoked' && before !== null && before > 1)
  }
  setHistoryReader(threadId: string, reader?: TrajectoryHistoryReader, throughSeq = 0): void {
    const entry = this.entry(threadId)
    this.cancelOlder(entry); entry.older = undefined
    if (Number.isSafeInteger(throughSeq) && throughSeq >= 0) entry.knownThroughSeq = Math.max(entry.knownThroughSeq, throughSeq)
    entry.reader = entry.projection.getSnapshot().history === 'revoked' ? undefined : reader
    entry.olderHistory = this.canReadOlder(entry) ? 'available' : 'unavailable'
    this.notify(threadId)
  }
  showLatest(threadId: string): void {
    const entry = this.entries.get(threadId)
    if (!entry) return
    this.cancelOlder(entry); entry.older = undefined
    entry.olderHistory = this.canReadOlder(entry) ? 'available' : 'unavailable'
    this.notify(threadId)
  }
  async loadOlder(threadId: string): Promise<boolean> {
    const entry = this.entries.get(threadId)
    if (!entry || entry.request || !entry.listeners.size || !this.canReadOlder(entry) || entry.olderHistory === 'exhausted') return false
    const before = this.olderBoundary(entry)!, through = Math.max(entry.knownThroughSeq, entry.projection.getSnapshot().loadedThroughSeq ?? 0)
    if (through <= 0) return false
    const reader = entry.reader!, request = new AbortController()
    const candidate = new TrajectoryProjection(threadId, 2000, before)
    entry.request = request; entry.requestThroughSeq = through; entry.olderHistory = 'loading'; this.notify(threadId)
    const current = () => this.entries.get(threadId) === entry && entry.request === request && !request.signal.aborted && !isPublicProjectionRevoked(threadId)
    try {
      await reader(through, events => {
        if (!current()) return
        if (!candidate.accept(events)) throw new Error('Invalid public trajectory history.')
        if (candidate.getSnapshot().history === 'revoked') { this.revoke(threadId); return }
        if (events.some(event => (event as { kind?: string }).kind === 'snapshot_required')) throw new Error('Public trajectory history is incomplete.')
      }, request.signal)
      if (!current()) return false
      candidate.complete(through)
      // An empty retained window can still have an older discarded prefix. Keep
      // its boundary so another request advances instead of replaying the same page.
      if (candidate.getSnapshot().records.length || candidate.getSnapshot().discardedThroughSeq !== null) entry.older = candidate
      entry.olderHistory = candidate.getSnapshot().discardedThroughSeq === null ? 'exhausted' : 'available'
      return true
    } catch {
      if (current()) entry.olderHistory = 'unavailable'
      return false
    } finally {
      if (entry.request === request) { entry.request = undefined; entry.requestThroughSeq = undefined; this.notify(threadId) }
    }
  }
  beginReplay(threadId: string, throughSeq: number) {
    const release = this.retainThread(threadId)
    const projection = this.entries.get(threadId)!.projection
    projection.loading(); this.notify(threadId)
    let settled = false, cursor = 0, valid = true
    return {
      accept: (events: readonly unknown[]) => {
        if (settled) return
        if (!isStrictPublicRuntimeSseIpcPayload({ streamId: 'trajectory-replay-observer', events }) ||
            events.some(value => (value as Record<string, unknown>).threadId !== threadId)) { valid = false; return }
        // The strict guard permits this single terminal control without a seq.
        // Runtime owns the shared revocation marker; this observer retires only its replay.
        if ((events[0] as Record<string, unknown> | undefined)?.kind === 'public_projection_revoked') {
          settled = true; this.revoke(threadId); release(); return
        }
        if (events.some(value => {
          const event = value as Record<string, unknown>
          return !Number.isSafeInteger(event.seq) || (event.seq as number) <= 0 || (event.seq as number) > throughSeq
        })) { valid = false; return }
        for (const event of events as Record<string, unknown>[]) cursor = Math.max(cursor, event.seq as number)
        this.observe(threadId, events)
      },
      complete: () => {
        if (settled) return
        settled = true
        if (valid && cursor === throughSeq) projection.complete(throughSeq); else projection.incomplete()
        this.notify(threadId); release()
      },
      fail: () => { if (!settled) { settled = true; projection.incomplete(); this.notify(threadId); release() } }
    }
  }
}
export const trajectorySource = new TrajectorySource()
