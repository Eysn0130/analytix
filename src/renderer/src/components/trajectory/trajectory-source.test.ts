import { describe, expect, it, vi } from 'vitest'
import { TrajectorySource } from './trajectory-source'
import { toolEvent, userEvent } from './trajectory-test-fixtures'

describe('trajectory transport observer', () => {
  it('requires finite replay completion at its own watermark, permits public seq gaps, deduplicates live/replay', () => {
    const source = new TrajectorySource()
    source.bindThread('trajectory-thread')
    source.observe('trajectory-thread', [userEvent(100)])
    const incomplete = source.beginReplay('trajectory-thread', 50)
    incomplete.accept([userEvent(1)]); incomplete.complete()
    expect(source.getSnapshot('trajectory-thread')).toMatchObject({ completeThroughSeq: null, history: 'partial' })
    const replay = source.beginReplay('trajectory-thread', 50)
    replay.accept([userEvent(1), userEvent(50)]); replay.complete()
    source.observe('trajectory-thread', [userEvent(50)])
    expect(source.getSnapshot('trajectory-thread')).toMatchObject({ completeThroughSeq: 50, loadedFromSeq: 1, loadedThroughSeq: 100, history: 'complete' })
    expect(source.getSnapshot('trajectory-thread').records).toHaveLength(3)
  })
  it('keeps unknown history distinct from empty and ignores callbacks after failure', () => {
    const source = new TrajectorySource()
    source.bindThread('trajectory-thread')
    expect(source.getSnapshot('trajectory-thread')).toMatchObject({ records: [], history: 'unknown', completeThroughSeq: null })
    const replay = source.beginReplay('trajectory-thread', 4)
    replay.accept([userEvent(1)]); replay.fail(); replay.accept([userEvent(4)]); replay.complete()
    expect(source.getSnapshot('trajectory-thread')).toMatchObject({ history: 'partial', completeThroughSeq: null, loadedThroughSeq: 1 })
  })
  it('isolates observers and thread switches without acquiring or stopping a transport', () => {
    const source = new TrajectorySource(), listener = vi.fn(), other = vi.fn()
    source.bindThread('trajectory-thread')
    const off = source.subscribe('trajectory-thread', listener)
    const bad = source.subscribe('trajectory-thread', () => { throw new Error('observer') })
    const otherOff = source.subscribe('other-thread', other)
    source.observe('trajectory-thread', [userEvent(1)])
    expect(listener).toHaveBeenCalledTimes(1); expect(other).not.toHaveBeenCalled()
    off(); bad(); otherOff()
    source.observe('trajectory-thread', [userEvent(2)])
    expect(listener).toHaveBeenCalledTimes(1)
    expect(source.getSnapshot('other-thread').records).toEqual([])
  })
  it('retires old bindings after their last real owner and ignores late old deliveries', () => {
    const source = new TrajectorySource()
    source.bindThread('trajectory-thread')
    source.observe('trajectory-thread', [userEvent(1)])
    const off = source.subscribe('trajectory-thread', () => undefined)
    source.bindThread('next-thread')
    expect(source.getSnapshot('trajectory-thread').records).toHaveLength(1)
    off()
    expect(source.getSnapshot('trajectory-thread').records).toEqual([])
    source.observe('trajectory-thread', [userEvent(2)])
    expect(source.getSnapshot('trajectory-thread').records).toEqual([])
    source.bindThread('trajectory-thread')
    expect(source.getSnapshot('trajectory-thread')).toMatchObject({ records: [], completeThroughSeq: null, history: 'unknown' })
  })

  it('accepts an unsequenced terminal replay revocation, purges old records and ignores late replay/live events', () => {
    const source = new TrajectorySource()
    source.bindThread('trajectory-thread')
    source.observe('trajectory-thread', [userEvent(1)])
    const replay = source.beginReplay('trajectory-thread', 10)
    replay.accept([{ schemaVersion: 1, kind: 'public_projection_revoked', threadId: 'trajectory-thread',
      historyAuthority: 'case_boundary_only_v1', code: 'case_public_authority_rejected', action: 'purge_case_projection', terminal: true }])
    expect(source.getSnapshot('trajectory-thread')).toMatchObject({ records: [], history: 'revoked', loadedThroughSeq: null, completeThroughSeq: null })
    replay.accept([userEvent(10)]); replay.fail(); replay.complete()
    source.observe('trajectory-thread', [userEvent(11)])
    expect(source.getSnapshot('trajectory-thread')).toMatchObject({ records: [], history: 'revoked', loadedThroughSeq: null, completeThroughSeq: null })
    source.bindThread('other-thread')
    expect(source.getSnapshot('trajectory-thread').history).toBe('unknown') // The settled replay lease has retired.
  })
  it.each(['foreign', 'malformed', 'mixed', 'sequenced'] as const)('rejects a %s replay control before changing records or cursors', mode => {
    const source = new TrajectorySource()
    source.bindThread('trajectory-thread')
    source.observe('trajectory-thread', [userEvent(1)])
    const replay = source.beginReplay('trajectory-thread', 10)
    const control = { schemaVersion: 1, kind: 'public_projection_revoked', threadId: mode === 'foreign' ? 'foreign-thread' : 'trajectory-thread',
      historyAuthority: 'case_boundary_only_v1', code: 'case_public_authority_rejected', action: 'purge_case_projection', terminal: mode !== 'malformed',
          ...(mode === 'sequenced' ? { seq: 10, timestamp: '2026-10-15T00:00:00Z' } : {}) }
    replay.accept(mode === 'mixed' ? [control, userEvent(10)] : [control]); replay.complete()
    expect(source.getSnapshot('trajectory-thread')).toMatchObject({ history: 'partial', loadedThroughSeq: 1, completeThroughSeq: null })
    expect(source.getSnapshot('trajectory-thread').records).toHaveLength(1)
  })

  it('loads successive bounded windows independently of live retention and returns to latest', async () => {
    const source = new TrajectorySource(), thread = 'trajectory-thread'
    source.bindThread(thread)
    const off = source.subscribe(thread, () => undefined)
    const events = Array.from({ length: 4001 }, (_, index) => userEvent(index + 1))
    // One more than the production live budget; both reads still replay the full log.
    source.observe(thread, events.slice(-2001))
    expect(source.getSnapshot(thread)).toMatchObject({ historyWindow: 'latest', truncated: true, discardedThroughSeq: 2001 })
    expect(source.getSnapshot(thread).records).toHaveLength(2000)
    const reader = vi.fn(async (through: number, accept: (events: readonly unknown[]) => void) => {
      expect(through).toBe(4001); accept(events)
    })
    source.setHistoryReader(thread, reader)
    expect(await source.loadOlder(thread)).toBe(true)
    expect(source.getSnapshot(thread)).toMatchObject({ historyWindow: 'older', olderHistory: 'available', completeThroughSeq: null, discardedThroughSeq: 1 })
    expect(source.getSnapshot(thread).records).toHaveLength(2000)
    expect(source.getSnapshot(thread).records[0].firstSeq).toBe(2)
    expect(await source.loadOlder(thread)).toBe(true)
    expect(source.getSnapshot(thread)).toMatchObject({ olderHistory: 'exhausted', completeThroughSeq: null })
    expect(source.getSnapshot(thread).records).toHaveLength(1)
    expect(await source.loadOlder(thread)).toBe(false)
    source.observe(thread, [userEvent(4002)])
    expect(source.getSnapshot(thread).records.at(-1)?.firstSeq).toBe(1)
    source.showLatest(thread)
    expect(source.getSnapshot(thread).records).toHaveLength(2000)
    expect(source.getSnapshot(thread).records.at(-1)?.firstSeq).toBe(4002)
    expect(reader).toHaveBeenCalledTimes(2)
    off()
  })
  it.each(['close', 'switch', 'revoke', 'rewind'] as const)('cancels an older request on %s and discards late success', async reason => {
    const source = new TrajectorySource(), thread = 'trajectory-thread'
    source.bindThread(thread)
    const off = source.subscribe(thread, () => undefined)
    source.observe(thread, [userEvent(10)])
    let accept!: (events: readonly unknown[]) => void, signal!: AbortSignal, resolve!: () => void
    source.setHistoryReader(thread, (_through, receive, currentSignal) => {
      accept = receive; signal = currentSignal
      return new Promise<void>(done => { resolve = done })
    })
    const pending = source.loadOlder(thread)
    expect(source.getSnapshot(thread).olderHistory).toBe('loading')
    if (reason === 'close') off()
    if (reason === 'switch') source.bindThread('other-thread')
    if (reason === 'revoke') source.revoke(thread)
    if (reason === 'rewind') source.observe(thread, [{ kind: 'thread_rewound', seq: 11, timestamp: '2026-10-15T00:00:11Z', threadId: thread,
      rewindTurnId: 'rewind-owner', removedTurns: 1, remainingTurns: 0, removedTurnIds: ['trajectory-turn'] }])
    expect(signal.aborted).toBe(true)
    accept([userEvent(1)]); resolve()
    expect(await pending).toBe(false)
    expect(source.getSnapshot(thread).records.some(record => record.firstSeq === 1)).toBe(false)
    expect(source.getSnapshot(thread).historyWindow).not.toBe('older')
    off()
  })
  it('keeps a failed candidate hidden, permits retry, and leaves unconfigured authority unavailable', async () => {
    const source = new TrajectorySource(), thread = 'trajectory-thread'
    source.bindThread(thread)
    const off = source.subscribe(thread, () => undefined)
    source.observe(thread, [userEvent(10)])
    expect(await source.loadOlder(thread)).toBe(false)
    source.setHistoryReader(thread, async (_through, accept) => { accept([userEvent(1)]); throw new Error('timeout') })
    expect(await source.loadOlder(thread)).toBe(false)
    expect(source.getSnapshot(thread)).toMatchObject({ olderHistory: 'unavailable', canLoadOlder: true, historyWindow: 'latest' })
    expect(source.getSnapshot(thread).records.map(record => record.firstSeq)).toEqual([10])
    source.setHistoryReader(thread)
    expect(source.getSnapshot(thread)).toMatchObject({ olderHistory: 'unavailable', canLoadOlder: false })
    off()
  })

  it('cancels a detached replay when a newer live finish would make its retained tool stale', async () => {
    const source = new TrajectorySource(), thread = 'trajectory-thread'
    source.bindThread(thread)
    const off = source.subscribe(thread, () => undefined)
    source.observe(thread, [toolEvent(false, 10)])
    let signal!: AbortSignal, resolve!: () => void
    source.setHistoryReader(thread, (_through, accept, currentSignal) => {
      signal = currentSignal; accept([toolEvent(false, 2)])
      return new Promise<void>(done => { resolve = done })
    })
    const pending = source.loadOlder(thread)
    source.observe(thread, [toolEvent(true, 11)])
    expect(signal.aborted).toBe(true)
    resolve()
    expect(await pending).toBe(false)
    expect(source.getSnapshot(thread)).toMatchObject({ historyWindow: 'latest' })
    expect(source.getSnapshot(thread).records[0]).toMatchObject({ status: 'completed', lastSeq: 11 })
    off()
  })

  it('uses the known snapshot head after partial ordinary replay and cannot lower it with older detail', async () => {
    const source = new TrajectorySource(), thread = 'trajectory-thread'
    source.bindThread(thread)
    const off = source.subscribe(thread, () => undefined)
    source.observe(thread, [userEvent(10)])
    const survivor = userEvent(2)
    const reader = vi.fn(async (through: number, accept: (events: readonly unknown[]) => void) => {
      expect(through).toBe(100)
      accept([userEvent(1), { ...survivor, turnId: 'survivor-turn', item: { ...survivor.item, turnId: 'survivor-turn' } }])
      accept([{ kind: 'thread_rewound', seq: 90, timestamp: '2026-10-15T00:00:09Z', threadId: thread,
        rewindTurnId: 'rewind-owner', removedTurns: 1, remainingTurns: 1, removedTurnIds: ['trajectory-turn'] },
      { kind: 'cursor_advanced', seq: 100, timestamp: '2026-10-15T00:00:10Z', threadId: thread, reason: 'restricted_content_removed' }])
    })
    source.setHistoryReader(thread, reader, 100)
    source.setHistoryReader(thread, reader, 80)
    expect(await source.loadOlder(thread)).toBe(true)
    expect(source.getSnapshot(thread).records.map(record => record.firstSeq)).toEqual([2])
    expect(source.getSnapshot(thread).loadedThroughSeq).toBe(100)
    off()
  })
  it('advances an empty window with a discarded prefix instead of repeating the same page', async () => {
    const source = new TrajectorySource(), thread = 'trajectory-thread'
    source.bindThread(thread)
    const off = source.subscribe(thread, () => undefined)
    const events = Array.from({ length: 4500 }, (_, index) => userEvent(index + 1))
    source.observe(thread, events)
    const reader = vi.fn(async (_through: number, accept: (events: readonly unknown[]) => void) => {
      accept(events)
      accept([{ kind: 'thread_rewound', seq: 4501, timestamp: '2026-10-15T00:00:09Z', threadId: thread,
        rewindTurnId: 'rewind-owner', removedTurns: 1, remainingTurns: 0, removedTurnIds: ['trajectory-turn'] }])
    })
    source.setHistoryReader(thread, reader, 4501)
    expect(await source.loadOlder(thread)).toBe(true)
    expect(source.getSnapshot(thread)).toMatchObject({ records: [], historyWindow: 'older', discardedThroughSeq: 500, olderHistory: 'available' })
    expect(await source.loadOlder(thread)).toBe(true)
    expect(source.getSnapshot(thread)).toMatchObject({ records: [], olderHistory: 'exhausted' })
    expect(await source.loadOlder(thread)).toBe(false)
    expect(reader).toHaveBeenCalledTimes(2)
    off()
  })

})
