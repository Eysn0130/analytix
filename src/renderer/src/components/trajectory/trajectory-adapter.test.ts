import { describe, expect, it } from 'vitest'
import { TrajectoryProjection, trajectoryDuration, trajectoryMatches } from './trajectory-adapter'
import { errorEvent, readyEvent, retryEvent, stepEvent, terminalErrorBatch, toolEvent, trajectoryTime, userEvent } from './trajectory-test-fixtures'

describe('public trajectory projection', () => {
  it('uses the assembly Host boundary once per logical step across retry/replay, never closed time', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    expect(projection.accept([stepEvent('logical_step_started', 2, 1)])).toBe(true)
    expect(projection.accept([{ kind: 'pipeline_stage', seq: 3, timestamp: trajectoryTime(2), threadId: 'trajectory-thread',
      turnId: 'trajectory-turn', stage: 'provider_retrying', label: 'Retrying provider stream', attempt: 2, maxAttempt: 3,
      details: { retry: { schemaVersion: 1, source: 'provider_retry', phase: 'started', logicalSequence: 1, outerAttempt: 2 } } }])).toBe(true)
    expect(projection.accept([stepEvent('logical_step_assembled', 4, 3), stepEvent('logical_step_closed', 8, 8)])).toBe(true)
    expect(projection.accept([stepEvent('logical_step_started', 2, 1), stepEvent('logical_step_assembled', 4, 3)])).toBe(true)
    const records = projection.getSnapshot().records
    expect(records).toHaveLength(2)
    expect(records[0]).toMatchObject({ firstSeq: 2, lastSeq: 8, startedAt: trajectoryTime(1), endedAt: trajectoryTime(3), status: 'completed' })
    expect(trajectoryDuration(records[0])).toBe(2000)
    expect(records[1]).toMatchObject({ kind: 'retry', firstSeq: 3, retry: { phase: 'started', attempt: 2, maxAttempt: 3 } })
    expect(records[1].logicalStepId).toBeUndefined()
  })
  it.each(['provider_retry', 'transport_reconnect'])('retains independent %s phases, zero delay and idempotent out-of-order replay', source => {
    const projection = new TrajectoryProjection('trajectory-thread')
    const events = ['scheduled', 'started', 'cancelled'].map((phase, index) => retryEvent(index + 2, phase, source))
    expect(projection.accept([stepEvent('logical_step_started', 1), ...[...events].reverse(), ...events])).toBe(true)
    const retries = projection.getSnapshot().records.filter(record => record.kind === 'retry')
    expect(retries).toHaveLength(3)
    expect(retries.map(record => record.retry?.phase)).toEqual(['scheduled', 'started', 'cancelled'])
    expect(retries[0].retry).toEqual({ source, phase: 'scheduled', attempt: source === 'transport_reconnect' ? 3 : 2, maxAttempt: 4, delayMs: 0 })
    for (const record of retries) {
      expect(record.logicalStepId).toBeUndefined(); expect(record.tool).toBeUndefined()
      expect(record.startedAt).toBeNull(); expect(record.endedAt).toBeNull(); expect(trajectoryDuration(record)).toBeNull()
      expect(record).not.toHaveProperty('details'); expect(record.retry).not.toHaveProperty('logicalSequence')
    }
  })
  it('retains legacy public retry counters without inventing a phase or delay', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    const event = { ...retryEvent(2), details: undefined }
    expect(projection.accept([event])).toBe(true)
    expect(projection.getSnapshot().records[0].retry).toEqual({ attempt: 2, maxAttempt: 4 })
  })
  it('retains closed error codes/details and public provider failure events independently', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    const provider = { kind: 'pipeline_stage', seq: 3, timestamp: trajectoryTime(3), threadId: 'trajectory-thread', turnId: 'trajectory-turn',
      stage: 'provider_error', label: 'Provider stream failed', attempt: 2, maxAttempt: 4,
      details: { reasonCode: 'provider_error', providerError: { code: 'provider_unavailable', kind: 'server', attempt: 2 } } }
    expect(projection.accept([errorEvent(2), provider, errorEvent(2)])).toBe(true)
    const records = projection.getSnapshot().records
    expect(records).toHaveLength(2)
    expect(records[0]).toMatchObject({ kind: 'error', status: 'failed', error: { code: 'provider_unavailable', phase: 'live', attempt: 2, maxAttempt: 4 } })
    expect(records[1]).toMatchObject({ kind: 'error', error: { code: 'provider_unavailable', attempt: 2, maxAttempt: 4 } })
    expect(trajectoryMatches(records[0], 'provider_unavailable live')).toBe(true)
    expect(records.every(record => record.logicalStepId === undefined && record.startedAt === null && record.endedAt === null)).toBe(true)
  })
  it.each([true, false])('retains atomic terminal errors once with error item=%s', withItem => {
    const projection = new TrajectoryProjection('trajectory-thread'), batch = terminalErrorBatch(withItem)
    expect(projection.accept([batch])).toBe(true)
    expect(projection.accept([batch])).toBe(true)
    expect(projection.getSnapshot().records).toHaveLength(1)
    expect(projection.getSnapshot().records[0]).toMatchObject({ kind: 'error', status: 'failed', error: { code: 'provider_unavailable' } })
  })
  it('preserves an aborted atomic error item as cancelled', () => {
    const projection = new TrajectoryProjection('trajectory-thread'), batch = terminalErrorBatch(true)
    const events = batch.events as Record<string, unknown>[]
    const cancelled = { code: 'turn_cancelled', message: 'The turn was cancelled before a verified response was available.', severity: 'warning' }
    Object.assign(events[0].item as Record<string, unknown>, cancelled, { status: 'aborted' })
    events[1].usageFinalStatus = 'aborted'
    events[2] = { ...events[2], ...cancelled, kind: 'turn_aborted', status: 'aborted', terminalReason: 'cancel', discard: false, cancelled: true, cancelledPendingGates: 0 }
    expect(projection.accept([batch])).toBe(true)
    expect(projection.getSnapshot().records[0]).toMatchObject({ kind: 'error', status: 'cancelled' })
  })
  it('rejects raw retry/error prose and unknown details before changing records or cursors', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    for (const event of [{ ...errorEvent(1), message: 'private provider body' }, { ...retryEvent(2), body: 'private body' },
      { ...errorEvent(3), details: { phase: 'live', raw: 'private body' } }]) expect(projection.accept([event])).toBe(false)
    expect(projection.getSnapshot()).toMatchObject({ records: [], loadedFromSeq: null, loadedThroughSeq: null })
  })
  it('keeps legacy end unknown even when closed has successful metrics and replay start arrives late', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    projection.accept([stepEvent('logical_step_closed', 8, 8)])
    projection.accept([stepEvent('logical_step_started', 2, 1)])
    const [record] = projection.getSnapshot().records
    expect(record).toMatchObject({ startedAt: trajectoryTime(1), endedAt: null, status: 'unknown' })
    expect(trajectoryDuration(record)).toBeNull()
  })
  it('keeps input a point and ready separate from actual tool execution, preserving real inspect identities', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    expect(projection.accept([userEvent(1), readyEvent(2)])).toBe(true)
    const [user, ready] = projection.getSnapshot().records
    expect(user).toMatchObject({ kind: 'user', at: trajectoryTime(0), startedAt: null, endedAt: null })
    expect(ready).toMatchObject({ status: 'ready', startedAt: null, endedAt: null })
    expect(projection.accept([toolEvent(false, 3), toolEvent(true, 7)])).toBe(true)
    const record = projection.getSnapshot().records[1]
    expect(record).toMatchObject({ startedAt: trajectoryTime(3), endedAt: trajectoryTime(7),
      tool: { threadId: 'trajectory-thread', turnId: 'trajectory-turn', callId: 'trajectory-call', toolCallItemId: 'trajectory-tool', toolResultItemId: 'trajectory-result' } })
    expect(trajectoryDuration(record)).toBe(4000)
    expect(record).not.toHaveProperty('parentCallId')
    expect(record).not.toHaveProperty('arguments')
    expect(record).not.toHaveProperty('output')
    expect(record.toolMetadata).toMatchObject({ argumentsWithheld: true, projectionKind: 'withheld' })
  })
  it('rejects whole foreign or non-public frames without storing protected data or advancing cursors', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    expect(projection.accept([userEvent(1), userEvent(2, 'foreign')])).toBe(false)
    expect(projection.accept([{ ...stepEvent('logical_step_started', 3), details: { schemaVersion: 1,
      logicalStepId: 'd560256d-4adc-401c-9c0a-b5f46052d513', reasoning: 'private sentinel' } }])).toBe(false)
    expect(projection.accept([{ ...toolEvent(false, 4), body: 'private sentinel' }])).toBe(false)
    expect(projection.getSnapshot()).toMatchObject({ records: [], loadedThroughSeq: null, completeThroughSeq: null })
  })
  it('indexes AND terms only from public display fields and permanently clears revoked history', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    projection.accept([userEvent(1, 'trajectory-thread', 'Alpha beta')])
    const [record] = projection.getSnapshot().records
    expect(trajectoryMatches(record, ' BETA alpha ')).toBe(true)
    expect(trajectoryMatches(record, 'alpha missing')).toBe(false)
    expect(projection.accept([{ schemaVersion: 1, kind: 'public_projection_revoked', threadId: 'trajectory-thread',
      historyAuthority: 'case_boundary_only_v1', code: 'case_public_authority_rejected', action: 'purge_case_projection', terminal: true }])).toBe(true)
    expect(projection.accept([userEvent(2)])).toBe(false)
    expect(projection.getSnapshot()).toMatchObject({ records: [], history: 'revoked', completeThroughSeq: null })
  })
  it('preserves a user item first sequence when live update precedes its replay creation', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    const original = userEvent(1)
    projection.accept([{ ...original, seq: 8, kind: 'item_updated', item: { ...original.item, text: 'Current public text' } }])
    projection.accept([original])
    expect(projection.getSnapshot().records).toHaveLength(1)
    expect(projection.getSnapshot().records[0]).toMatchObject({ firstSeq: 1, lastSeq: 8, text: 'Current public text' })
  })
  it('bounds retained records and refuses to revive a discarded prefix or claim complete search', () => {
    const projection = new TrajectoryProjection('trajectory-thread', 2)
    projection.accept([userEvent(1), userEvent(2), userEvent(3)])
    projection.complete(3)
    expect(projection.getSnapshot()).toMatchObject({ truncated: true, discardedThroughSeq: 1, completeThroughSeq: null, loadedFromSeq: 2 })
    expect(projection.getSnapshot().records.map(record => record.firstSeq)).toEqual([2, 3])
    projection.accept([userEvent(1)])
    expect(projection.getSnapshot().records.map(record => record.firstSeq)).toEqual([2, 3])
  })
  it('bounds public text previews and keeps truncation attributable to the visible record', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    projection.accept(Array.from({ length: 12 }, (_, index) => userEvent(index + 1, 'trajectory-thread', 'a'.repeat(20000) + 'omitted-suffix')))
    const snapshot = projection.getSnapshot()
    expect(snapshot.records.reduce((total, record) => total + (record.text?.length ?? 0), 0)).toBeLessThanOrEqual(128 * 1024)
    expect(snapshot.records.every(record => (record.text?.length ?? 0) <= 16 * 1024)).toBe(true)
    expect(snapshot.truncated).toBe(true)
    expect(snapshot.records.some(record => trajectoryMatches(record, 'omitted-suffix'))).toBe(false)
  })

  it('keeps a bounded older window, applies later updates and rewinds, and cannot claim full-history search', () => {
    const projection = new TrajectoryProjection('trajectory-thread', 2, 5)
    projection.accept([userEvent(1), userEvent(2), userEvent(3), userEvent(4), userEvent(5)])
    expect(projection.getSnapshot().records.map(record => record.firstSeq)).toEqual([3, 4])
    projection.accept([{ ...userEvent(3), seq: 6, kind: 'item_updated', item: { ...userEvent(3).item, text: 'Current public text' } }])
    expect(projection.getSnapshot().records[0]).toMatchObject({ firstSeq: 3, lastSeq: 6, text: 'Current public text' })
    projection.accept([{ kind: 'thread_rewound', seq: 7, timestamp: trajectoryTime(7), threadId: 'trajectory-thread',
      rewindTurnId: 'rewind-owner', removedTurns: 1, remainingTurns: 0, removedTurnIds: ['trajectory-turn'] }])
    projection.accept([userEvent(8)])
    projection.complete(8)
    projection.accept([userEvent(3)]) // A late replay cannot resurrect a rewound old record after publication.
    expect(projection.getSnapshot()).toMatchObject({ records: [], history: 'partial', completeThroughSeq: null })
  })
  it('admits an entire atomic terminal delivery that crosses the older window boundary', () => {
    const projection = new TrajectoryProjection('trajectory-thread', 2, 10)
    expect(projection.accept([terminalErrorBatch(true)])).toBe(true)
    expect(projection.getSnapshot().records).toHaveLength(1)
    expect(projection.getSnapshot().records[0]).toMatchObject({ kind: 'error', firstSeq: 9, status: 'failed' })
    projection.complete(11)
    expect(projection.getSnapshot().completeThroughSeq).toBeNull()
  })

})
