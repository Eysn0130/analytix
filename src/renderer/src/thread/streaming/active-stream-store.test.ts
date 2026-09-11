import { beforeEach, describe, expect, it } from 'vitest'
import {
  appendActiveStreamDeltas,
  clearActiveStream,
  getActiveStreamEstimateMetricsFor,
  getActiveStreamMetricsFor,
  getActiveStreamSnapshot,
  getActiveStreamSnapshotFor,
  migrateActiveStreamTurn,
  pendingActiveStreamTurnId,
  resetActiveStream
} from './active-stream-store'

describe('active-stream-store', () => {
  beforeEach(() => {
    clearActiveStream()
  })

  it('tracks cursors by thread and turn without retaining assistant draft bytes', () => {
    appendActiveStreamDeltas({
      threadId: 'thread-a',
      turnId: 'turn-a',
      assistant: 'ACCOUNT 6222021234567890',
      lastSeq: 1
    })
    appendActiveStreamDeltas({
      threadId: 'thread-a',
      turnId: 'turn-b',
      assistant: 'other draft',
      lastSeq: 2
    })
    appendActiveStreamDeltas({
      threadId: 'thread-a',
      turnId: 'turn-a',
      assistant: '</think>late public',
      lastSeq: 3
    })

    expect(getActiveStreamSnapshotFor('thread-a', 'turn-a')).toMatchObject({
      threadId: 'thread-a',
      turnId: 'turn-a',
      liveAssistant: '',
      liveAssistantContent: '',
      lastSeq: 3
    })
    expect(getActiveStreamSnapshotFor('thread-a', 'turn-b')).toMatchObject({
      threadId: 'thread-a',
      turnId: 'turn-b',
      liveAssistant: '',
      lastSeq: 2
    })
    expect(getActiveStreamSnapshot()).toMatchObject({ threadId: 'thread-a', turnId: 'turn-a' })
  })

  it('does not restore draft text through reset', () => {
    resetActiveStream('thread-a', {
      turnId: 'turn-a',
      liveAssistant: 'unaccepted snapshot',
      lastSeq: 5,
      startedAt: 100
    })

    expect(getActiveStreamSnapshotFor('thread-a', 'turn-a')).toMatchObject({
      liveAssistant: '',
      liveAssistantContent: '',
      lastSeq: 5,
      startedAt: 100
    })
  })

  it('migrates cursor state without migrating candidate bytes', () => {
    const pendingTurnId = pendingActiveStreamTurnId('thread-a', 'user-a')
    appendActiveStreamDeltas({
      threadId: 'thread-a',
      turnId: pendingTurnId,
      assistant: '<think>private</think>fabricated fact',
      lastSeq: 9,
      startedAt: 100
    })

    migrateActiveStreamTurn('thread-a', pendingTurnId, 'turn-a')

    expect(getActiveStreamSnapshotFor('thread-a', pendingTurnId).threadId).toBeNull()
    expect(getActiveStreamSnapshotFor('thread-a', 'turn-a')).toMatchObject({
      threadId: 'thread-a',
      turnId: 'turn-a',
      liveAssistant: '',
      liveAssistantContent: '',
      lastSeq: 9,
      startedAt: 100
    })
  })

  it('reports zero assistant-content metrics for withheld drafts', () => {
    appendActiveStreamDeltas({
      threadId: 'thread-a',
      turnId: 'turn-a',
      assistant: 'a'.repeat(512),
      lastSeq: 4
    })

    expect(getActiveStreamMetricsFor('thread-a', 'turn-a')).toMatchObject({
      assistantLength: 0,
      processLength: 0,
      contentLength: 0,
      lastSeq: 4
    })
    expect(getActiveStreamEstimateMetricsFor('thread-a', 'turn-a')).toMatchObject({
      processBucket: 0,
      contentBucket: 0
    })
  })

  it('clears one turn without dropping another cursor', () => {
    appendActiveStreamDeltas({ threadId: 'thread-a', turnId: 'turn-a', lastSeq: 1 })
    appendActiveStreamDeltas({ threadId: 'thread-a', turnId: 'turn-b', lastSeq: 2 })

    clearActiveStream('thread-a', 'turn-a')
    expect(getActiveStreamSnapshotFor('thread-a', 'turn-a').threadId).toBeNull()
    expect(getActiveStreamSnapshotFor('thread-a', 'turn-b')).toMatchObject({
      threadId: 'thread-a',
      turnId: 'turn-b',
      lastSeq: 2
    })

    clearActiveStream('thread-a')
    expect(getActiveStreamSnapshotFor('thread-a', 'turn-b').threadId).toBeNull()
  })
})
