import { isStrictPublicRuntimeSseIpcPayload } from '@shared/public-runtime-sse'

export type TrajectoryLane = 'input' | 'model' | 'tool'
export type TrajectoryToolIdentity = {
  threadId: string; turnId: string; callId: string
  toolCallItemId?: string; toolResultItemId?: string
}
export type TrajectoryRecord = Readonly<{
  id: string; threadId: string; turnId: string; lane: TrajectoryLane
  kind: 'user' | 'step' | 'tool' | 'message' | 'compaction' | 'retry' | 'error'
  firstSeq: number; lastSeq: number; startSeq?: number; endSeq?: number; closeSeq?: number; title: string; status: string
  at: string | null; startedAt: string | null; endedAt: string | null
  text?: string; textTruncated?: boolean
  metrics?: Readonly<Partial<Record<'llmMs' | 'ttftMs' | 'decodeMs' | 'decodeTokens', number>>>
  toolMetadata?: Readonly<{ argumentsWithheld?: boolean; projectionKind?: string; messageKey?: string; code?: string }>
  retry?: Readonly<{ source?: string; phase?: string; attempt?: number; maxAttempt?: number; delayMs?: number }>
  error?: Readonly<{ code?: string; phase?: string; attempt?: number; maxAttempt?: number }>
  logicalStepId?: string; logicalClosed?: boolean; tool?: TrajectoryToolIdentity; ambiguousTool?: boolean
}>
export type TrajectorySnapshot = Readonly<{
  threadId: string; records: readonly TrajectoryRecord[]
  loadedFromSeq: number | null; loadedThroughSeq: number | null
  completeThroughSeq: number | null
  truncated: boolean; discardedThroughSeq: number | null
  history: 'unknown' | 'loading' | 'partial' | 'complete' | 'revoked'
  historyWindow?: 'latest' | 'older'
  olderHistory?: 'available' | 'loading' | 'unavailable' | 'exhausted'
  canLoadOlder?: boolean
}>
type PublicEvent = Record<string, unknown>
const object = (value: unknown): PublicEvent | null => value !== null && typeof value === 'object' && !Array.isArray(value) ? value as PublicEvent : null
const string = (value: unknown): string | undefined => typeof value === 'string' && value.length > 0 ? value : undefined
const hostTime = (value: unknown): string | null => typeof value === 'string' && Number.isFinite(Date.parse(value)) ? value : null

/** Whitelisted facts only. Stable record IDs and field seqs make replay/live idempotent; no seen-event log is retained. */
export class TrajectoryProjection {
  private records = new Map<string, TrajectoryRecord>()
  private discardedThroughSeq = 0
  private windowSealed = false
  private removedTurns = new Map<string, number>()
  private snapshot: TrajectorySnapshot
  constructor(readonly threadId: string, private readonly maxRecords = 2000, private readonly beforeSeq = Infinity) {
    this.snapshot = { threadId, records: [], loadedFromSeq: null, loadedThroughSeq: null, completeThroughSeq: null, truncated: false, discardedThroughSeq: null, history: 'unknown' }
  }
  getSnapshot = (): TrajectorySnapshot => this.snapshot
  private publish(patch: Partial<TrajectorySnapshot> = {}): void {
    let records = [...this.records.values()].sort((a, b) => a.firstSeq - b.firstSeq || a.id.localeCompare(b.id))
    // A fixed current-thread budget, not a cross-thread cache or an event archive.
    let floor = this.discardedThroughSeq
    if (records.length > this.maxRecords) {
      const oldest = [...records].sort((a, b) => a.lastSeq - b.lastSeq)
      floor = Math.max(floor, oldest[oldest.length - this.maxRecords - 1].lastSeq)
    }
    if (this.removedTurns.size > this.maxRecords) {
      const tombstones = [...this.removedTurns.values()].sort((a, b) => a - b)
      floor = Math.max(floor, tombstones[tombstones.length - this.maxRecords - 1])
    }
    if (floor > this.discardedThroughSeq) this.discardedThroughSeq = floor
    if (floor > 0) {
      for (const record of records) if (record.lastSeq <= floor) this.records.delete(record.id)
      for (const [turnId, seq] of this.removedTurns) if (seq <= floor) this.removedTurns.delete(turnId)
      records = records.filter(record => record.lastSeq > floor)
    }
    let textRemaining = 128 * 1024
    for (let index = records.length - 1; index >= 0; index--) {
      const record = records[index]
      if (record.text === undefined) continue
      const keep = Math.min(record.text.length, 16 * 1024, textRemaining)
      textRemaining -= keep
      if (keep < record.text.length) {
        const shortened = { ...record, text: record.text.slice(0, keep), textTruncated: true }
        this.records.set(record.id, shortened); records[index] = shortened
      }
    }
    const truncated = floor > 0 || records.some(record => record.textTruncated)
    this.snapshot = { ...this.snapshot, ...patch, records,
      ...(truncated ? { truncated: true, completeThroughSeq: null, discardedThroughSeq: floor || null,
        loadedFromSeq: records[0]?.firstSeq ?? null, history: patch.history === 'revoked' ? 'revoked' : 'partial' } : {}) }
  }
  loading(): void { if (this.snapshot.history !== 'revoked') this.publish({ history: 'loading' }) }
  incomplete(): void { if (this.snapshot.history !== 'revoked') this.publish({ history: 'partial' }) }
  complete(throughSeq: number): void {
    // A historical window deliberately excludes newer records, even after a full replay.
    if (Number.isFinite(this.beforeSeq)) { this.windowSealed = true; this.incomplete(); return }
    if (this.snapshot.history !== 'revoked' && !this.snapshot.truncated && Number.isSafeInteger(throughSeq) && throughSeq >= 0 &&
        (throughSeq === 0 || (this.snapshot.loadedThroughSeq ?? -1) >= throughSeq)) {
      this.publish({ completeThroughSeq: Math.max(this.snapshot.completeThroughSeq ?? 0, throughSeq), history: 'complete' })
    }
  }
  revoke(): void {
    this.records.clear(); this.removedTurns.clear(); this.discardedThroughSeq = 0
    this.publish({ history: 'revoked', loadedFromSeq: null, loadedThroughSeq: null, completeThroughSeq: null, truncated: false, discardedThroughSeq: null })
  }
  accept(events: readonly unknown[]): boolean {
    if (this.snapshot.history === 'revoked' || !isStrictPublicRuntimeSseIpcPayload({ streamId: 'trajectory-observer', events })) return false
    // Reject the entire foreign delivery before changing even a cursor.
    if (events.some(value => object(value)?.threadId !== this.threadId)) return false
    for (const value of events) {
      const event = value as PublicEvent
      if (event.kind === 'public_projection_revoked') { this.revoke(); return true }
      const seq = event.seq as number
      if (!Number.isSafeInteger(seq) || seq <= this.discardedThroughSeq) continue
      const from = typeof event.firstSeq === 'number' ? event.firstSeq : seq
      this.snapshot = { ...this.snapshot, loadedFromSeq: Math.min(this.snapshot.loadedFromSeq ?? from, from),
        loadedThroughSeq: Math.max(this.snapshot.loadedThroughSeq ?? 0, seq) }
      if (event.kind === 'snapshot_required') { this.incomplete(); continue }
      if (event.kind === 'accepted_final_batch' || event.kind === 'general_terminal_batch') {
        // The owner already validated and committed this atomic delivery. Nested events
        // are projected once; they never become independently dispatched/ACKed events.
        for (const nested of event.events as PublicEvent[]) this.fold(nested, true, !this.windowSealed && from < this.beforeSeq)
      } else this.fold(event, false, !this.windowSealed && from < this.beforeSeq)
    }
    this.publish()
    return true
  }
  private fold(event: PublicEvent, atomic: boolean, admit: boolean): void {
    const turnId = string(event.turnId)
    const seq = event.seq as number
    if (seq <= this.discardedThroughSeq) return
    if (event.kind === 'thread_rewound') {
      const removed = new Set(event.removedTurnIds as string[] ?? [])
      for (const [id, record] of this.records) if (removed.has(record.turnId)) this.records.delete(id)
      // After the window boundary, new records cannot enter. Retain only prefix
      // tombstones; later rewinds still delete every affected resident record.
      if (admit) for (const id of removed) this.removedTurns.set(id, seq)
      return
    }
    if (!turnId || this.removedTurns.has(turnId)) return
    const time = hostTime(event.timestamp)
    const item = object(event.item)
    const details = object(event.details)
    const logicalStepId = string(details?.logicalStepId)
    let id: string, base: TrajectoryRecord
    if (event.kind === 'pipeline_stage' && event.stage === 'provider_retrying') {
      const retry = object(details?.retry)
      // The public retry wire has no step UUID. Keep each real phase event independent.
      id = JSON.stringify(['retry', turnId, seq])
      if (!admit && !this.records.has(id)) return
      if (this.records.has(id)) return
      this.records.set(id, { id, threadId: this.threadId, turnId, lane: 'model', kind: 'retry', firstSeq: seq, lastSeq: seq,
        title: 'Retry', status: retry?.phase === 'started' ? 'running' : retry?.phase === 'cancelled' ? 'cancelled' : 'recorded',
        at: time, startedAt: null, endedAt: null,
        retry: { ...(typeof retry?.source === 'string' ? { source: retry.source } : {}),
          ...(typeof retry?.phase === 'string' ? { phase: retry.phase } : {}),
          ...(typeof event.attempt === 'number' ? { attempt: event.attempt } : {}),
          ...(typeof event.maxAttempt === 'number' ? { maxAttempt: event.maxAttempt } : {}),
          ...(typeof retry?.delayMs === 'number' ? { delayMs: retry.delayMs } : {}) } })
      return
    }
    if (event.kind === 'error' || event.kind === 'pipeline_stage' && event.stage === 'provider_error' ||
        atomic && (item?.kind === 'error' || ['turn_failed', 'turn_aborted'].includes(String(event.kind)) && !event.itemId)) {
      const error = item?.kind === 'error' ? item : event
      const errorDetails = object(error.details), providerError = object(errorDetails?.providerError)
      const code = string(error.code) ?? string(providerError?.code) ?? string(errorDetails?.reasonCode)
      const phase = string(errorDetails?.phase)
      const attempt = errorDetails?.attempt ?? providerError?.attempt ?? event.attempt, maxAttempt = errorDetails?.maxAttempt ?? event.maxAttempt
      id = JSON.stringify(['error', turnId, item?.kind === 'error' ? item.id : seq])
      if (!admit && !this.records.has(id)) return
      const previous = this.records.get(id)
      if (previous && previous.lastSeq >= seq) return
      this.records.set(id, { id, threadId: this.threadId, turnId, lane: 'model', kind: 'error',
        firstSeq: Math.min(previous?.firstSeq ?? seq, seq), lastSeq: seq, title: 'Error',
        status: item?.kind === 'error' ? item.status === 'aborted' ? 'cancelled' : String(item.status)
          : event.kind === 'turn_aborted' ? 'cancelled' : 'failed', at: time, startedAt: null, endedAt: null,
        error: { ...(code ? { code } : {}), ...(phase ? { phase } : {}),
          ...(typeof attempt === 'number' ? { attempt } : {}), ...(typeof maxAttempt === 'number' ? { maxAttempt } : {}) },
        ...(typeof error.message === 'string' ? { text: error.message } : typeof event.label === 'string' ? { text: event.label } : {}) })
      return
    }
    if (event.kind === 'pipeline_stage' && logicalStepId && details?.schemaVersion === 1 &&
        ['logical_step_started', 'logical_step_assembled', 'logical_step_closed'].includes(String(event.stage))) {
      id = JSON.stringify(['step', turnId, logicalStepId])
      if (!admit && !this.records.has(id)) return
      base = this.records.get(id) ?? { id, threadId: this.threadId, turnId, lane: 'model', kind: 'step',
        logicalStepId, firstSeq: seq, lastSeq: seq, title: 'Model', status: 'unknown', at: null, startedAt: null, endedAt: null }
      // closed is a metrics boundary, potentially after tools; never use it as end.
      this.records.set(id, { ...base, firstSeq: Math.min(base.firstSeq, seq), lastSeq: Math.max(base.lastSeq, seq),
        ...(event.stage === 'logical_step_started' && (base.startSeq === undefined || seq < base.startSeq) ? { startSeq: seq, at: time, startedAt: time, status: base.endedAt ? 'completed' : base.logicalClosed ? 'unknown' : 'running' } : {}),
        ...(event.stage === 'logical_step_assembled' && (base.endSeq === undefined || seq < base.endSeq) ? { endSeq: seq, endedAt: time, status: 'completed' } : {}),
        ...(event.stage === 'logical_step_closed' && (base.closeSeq === undefined || seq > base.closeSeq) ? { closeSeq: seq, logicalClosed: true, metrics: Object.fromEntries(
          ['llmMs', 'ttftMs', 'decodeMs', 'decodeTokens'].flatMap(key => typeof details[key] === 'number' ? [[key, details[key]]] : [])), ...!base.endedAt ? { status: 'unknown' } : {} } : {}) })
      return
    }
    const callId = string(event.callId) ?? string(item?.callId)
    if (callId && (event.kind === 'tool_call_ready' || item && ['tool_call', 'tool_result'].includes(String(item.kind)))) {
      id = JSON.stringify(['tool', turnId, callId])
      if (!admit && !this.records.has(id)) return
      base = this.records.get(id) ?? { id, threadId: this.threadId, turnId, lane: 'tool', kind: 'tool', firstSeq: seq, lastSeq: seq,
        title: string(event.toolName) ?? string(item?.toolName) ?? 'Tool', status: 'ready', at: time, startedAt: null, endedAt: null,
        tool: { threadId: this.threadId, turnId, callId } }
      const started = event.kind === 'tool_call_started' && item?.kind === 'tool_call'
      const finished = event.kind === 'tool_call_finished' && item?.kind === 'tool_result'
      const newer = seq >= base.lastSeq
      const conflict = item?.kind === 'tool_call' && base.tool?.toolCallItemId && base.tool.toolCallItemId !== item.id ||
        item?.kind === 'tool_result' && base.tool?.toolResultItemId && base.tool.toolResultItemId !== item.id
      this.records.set(id, { ...base, firstSeq: Math.min(base.firstSeq, seq), lastSeq: Math.max(base.lastSeq, seq),
        ambiguousTool: base.ambiguousTool || Boolean(conflict),
        toolMetadata: { ...base.toolMetadata, ...(item?.kind === 'tool_call' ? { argumentsWithheld: true } : {}),
          ...(item?.kind === 'tool_result' && (newer || !base.toolMetadata?.projectionKind) ? Object.fromEntries(['projectionKind', 'messageKey', 'code'].flatMap(key => {
            const value = object(item.output)?.[key]; return typeof value === 'string' ? [[key, value]] : []
          })) : {}) },
        ...(started && (base.startSeq === undefined || seq < base.startSeq) ? { startSeq: seq, startedAt: time, at: time } : {}),
        ...(finished && (base.endSeq === undefined || seq > base.endSeq) ? { endSeq: seq, endedAt: time } : {}),
        ...(newer ? { status: finished ? string(object(item?.output)?.status) ?? string(item?.status) ?? 'unknown' : started ? 'running' : base.status } : {}),
        tool: { ...base.tool!, ...(item?.kind === 'tool_call' ? { toolCallItemId: string(item.id) } : {}),
          ...(item?.kind === 'tool_result' ? { toolResultItemId: string(item.id) } : {}) } })
      return
    }
    if (item?.kind === 'compaction' || ['compaction_started', 'compaction_completed', 'compaction_failed'].includes(String(event.kind))) {
      const compactionId = string(item?.id) ?? string(event.compactionId)
      if (!compactionId) return
      id = JSON.stringify(['compaction', turnId, compactionId])
      if (!admit && !this.records.has(id)) return
      const previous = this.records.get(id)
      const text = item?.summary ?? event.summary
      if (previous && previous.lastSeq >= seq) { this.records.set(id, { ...previous, firstSeq: Math.min(previous.firstSeq, seq) }); return }
      this.records.set(id, { id, threadId: this.threadId, turnId, lane: 'input', kind: 'compaction',
        firstSeq: Math.min(previous?.firstSeq ?? seq, seq), lastSeq: Math.max(previous?.lastSeq ?? seq, seq),
        title: 'Context compacted', status: event.kind === 'compaction_started' ? 'running' : event.kind === 'compaction_failed' ? 'failed' : 'completed',
        at: hostTime(item?.createdAt) ?? time, startedAt: null, endedAt: null,
        ...(typeof text === 'string' ? { text } : {}) })
      return
    }
    if (item && item.threadId === this.threadId && item.turnId === turnId && string(item.id) &&
        (item.kind === 'user_message' || atomic && item.kind === 'assistant_text')) {
      const user = item.kind === 'user_message'
      id = JSON.stringify([user ? 'user' : 'message', turnId, item.id])
      if (!admit && !this.records.has(id)) return
      const previous = this.records.get(id)
      if (previous && previous.lastSeq >= seq) {
        this.records.set(id, { ...previous, firstSeq: Math.min(previous.firstSeq, seq) })
        return
      }
      this.records.set(id, { id, threadId: this.threadId, turnId, lane: user ? 'input' : 'model', kind: user ? 'user' : 'message',
        firstSeq: previous?.firstSeq ?? seq, lastSeq: seq, title: user ? 'User' : 'Assistant message', status: 'recorded',
        at: hostTime(item.createdAt), startedAt: null, endedAt: null,
        ...(typeof (item.displayText ?? item.text) === 'string' ? { text: (item.displayText ?? item.text) as string } : {}) })
    }
  }
}

/** AND terms, from a closed public whitelist; no arguments, output, JSON or protected text. */
export function trajectoryMatches(record: TrajectoryRecord, query: string): boolean {
  const haystack = [record.title, record.kind, record.status, record.turnId, record.logicalStepId,
    record.tool?.callId, record.text, record.retry?.phase, record.error?.code, record.error?.phase].filter(Boolean).join(' ').toLocaleLowerCase()
  return query.toLocaleLowerCase().trim().split(/\s+/u).filter(Boolean).every(term => haystack.includes(term))
}
export function trajectoryDuration(record: TrajectoryRecord): number | null {
  if (!record.startedAt || !record.endedAt) return null
  const duration = Date.parse(record.endedAt) - Date.parse(record.startedAt)
  return duration >= 0 ? duration : null
}
