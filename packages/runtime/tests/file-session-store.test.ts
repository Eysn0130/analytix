import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import type { UsageSnapshot } from '../src/contracts/usage.js'

const atomicWriteFileMock = vi.hoisted(() => vi.fn())

vi.mock('../src/adapters/file/atomic-write.js', () => ({
  atomicWriteFile: atomicWriteFileMock
}))

const { FileSessionStore } = await import('../src/adapters/file/file-session-store.js')

describe('FileSessionStore', () => {
  let dataDir = ''
  let warnSpy: ReturnType<typeof vi.spyOn>

  beforeEach(async () => {
    dataDir = await mkdtemp(join(tmpdir(), 'analytix-session-'))
    atomicWriteFileMock.mockReset()
    atomicWriteFileMock.mockResolvedValue(undefined)
    warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  })

  afterEach(async () => {
    warnSpy.mockRestore()
    await rm(dataDir, { recursive: true, force: true })
  })

  it('replays events.jsonl in seq order while ignoring malformed lines', async () => {
    const sessionStore = new FileSessionStore({ dataDir })
    const threadId = 'thr_replay_jsonl'
    const threadDir = join(dataDir, 'threads', threadId)
    await mkdir(threadDir, { recursive: true })
    await writeFile(join(threadDir, 'events.jsonl'), [
      JSON.stringify({
        kind: 'heartbeat',
        threadId,
        seq: 7,
        timestamp: '2026-06-03T00:00:07.000Z'
      }),
      '{not-json',
      JSON.stringify({
        kind: 'heartbeat',
        threadId,
        seq: 3,
        timestamp: '2026-06-03T00:00:03.000Z'
      }),
      JSON.stringify({
        kind: 'heartbeat',
        threadId,
        seq: 5,
        timestamp: '2026-06-03T00:00:05.000Z'
      })
    ].join('\n'), 'utf-8')

    await expect(sessionStore.highestSeq(threadId)).resolves.toBe(7)
    await expect(sessionStore.loadEventsSince(threadId, 4)).resolves.toMatchObject([
      { seq: 5 },
      { seq: 7 }
    ])
  })

  it('keeps event replay and highestSeq current after the events cache is warm', async () => {
    const sessionStore = new FileSessionStore({ dataDir })
    const threadId = 'thr_event_cache'
    await sessionStore.appendEvent(threadId, {
      kind: 'heartbeat',
      threadId,
      seq: 2,
      timestamp: '2026-06-03T00:00:02.000Z'
    })
    await expect(sessionStore.loadEventsSince(threadId, 0)).resolves.toMatchObject([{ seq: 2 }])
    await expect(sessionStore.highestSeq(threadId)).resolves.toBe(2)

    await sessionStore.appendEvent(threadId, {
      kind: 'heartbeat',
      threadId,
      seq: 5,
      timestamp: '2026-06-03T00:00:05.000Z'
    })

    await expect(sessionStore.highestSeq(threadId)).resolves.toBe(5)
    await expect(sessionStore.loadEventsSince(threadId, 2)).resolves.toMatchObject([{ seq: 5 }])

    await sessionStore.resetMemory()
    await expect(sessionStore.highestSeq(threadId)).resolves.toBe(5)
    await expect(sessionStore.loadEventsSince(threadId, 0)).resolves.toMatchObject([
      { seq: 2 },
      { seq: 5 }
    ])
  })

  it('invalidates warm event caches after usage compaction rewrites events.jsonl', async () => {
    atomicWriteFileMock.mockImplementation(async (path: string, contents: string) => {
      await writeFile(path, contents, 'utf-8')
    })
    const sessionStore = new FileSessionStore({
      dataDir,
      usageEventCompaction: {
        maxBytes: 1,
        retentionDays: 365,
        nowIso: () => '2026-06-03T00:00:00.000Z'
      }
    })
    const threadId = 'thr_usage_cache_compact'
    const usage = (tokens: number): UsageSnapshot => ({
      promptTokens: tokens,
      completionTokens: 0,
      totalTokens: tokens,
      cacheHitRate: null,
      turns: tokens
    })

    await sessionStore.appendEvent(threadId, {
      kind: 'usage',
      seq: 1,
      timestamp: '2025-06-04T00:00:00.000Z',
      threadId,
      providerId: 'deepseek',
      model: 'shared-model',
      usage: usage(1)
    })
    await expect(sessionStore.loadEventsSince(threadId, 0)).resolves.toMatchObject([{ seq: 1 }])
    await expect(sessionStore.highestSeq(threadId)).resolves.toBe(1)

    await sessionStore.appendEvent(threadId, {
      kind: 'usage',
      seq: 2,
      timestamp: '2025-06-04T01:00:00.000Z',
      threadId,
      providerId: 'deepseek',
      model: 'shared-model',
      usage: usage(2)
    })

    await expect(sessionStore.highestSeq(threadId)).resolves.toBe(2)
    await expect(sessionStore.loadEventsSince(threadId, 0)).resolves.toMatchObject([{ seq: 2 }])
    await expect(sessionStore.loadEventsSince(threadId, 1)).resolves.toMatchObject([{ seq: 2 }])
  })

  it('keeps appended usage events and projects compaction failures without private values', async () => {
    const privateThreadSentinel = 'thr_SLICE49_PRIVATE_THREAD_SENTINEL'
    const privatePathSentinel = 'SLICE49_PRIVATE_PATH_SENTINEL'
    const sessionStore = new FileSessionStore({
      dataDir,
      usageEventCompaction: {
        maxBytes: 1,
        retentionDays: 365,
        nowIso: () => '2026-06-03T00:00:00.000Z'
      }
    })
    const usage = (tokens: number): UsageSnapshot => ({
      promptTokens: tokens,
      completionTokens: 0,
      totalTokens: tokens,
      cacheHitRate: null,
      turns: tokens
    })

    await sessionStore.appendEvent(privateThreadSentinel, {
      kind: 'usage',
      seq: 1,
      timestamp: '2024-01-01T00:00:00.000Z',
      threadId: privateThreadSentinel,
      model: 'deepseek-chat',
      usage: usage(1)
    })
    await sessionStore.appendEvent(privateThreadSentinel, {
      kind: 'usage',
      seq: 2,
      timestamp: '2025-06-04T00:00:00.000Z',
      threadId: privateThreadSentinel,
      model: 'deepseek-chat',
      usage: usage(2)
    })

    const error = new Error(`/private/${privatePathSentinel}/events.jsonl`) as Error & { code: string }
    error.code = 'EPERM'
    atomicWriteFileMock.mockRejectedValueOnce(error)

    await expect(sessionStore.appendEvent(privateThreadSentinel, {
      kind: 'usage',
      seq: 3,
      timestamp: '2025-06-04T01:00:00.000Z',
      threadId: privateThreadSentinel,
      model: 'deepseek-chat',
      usage: usage(3)
    })).resolves.toBeUndefined()

    const events = await sessionStore.loadEventsSince(privateThreadSentinel, 0)
    expect(events.map((event) => event.seq)).toEqual([1, 2, 3])
    expect(atomicWriteFileMock).toHaveBeenCalledTimes(1)
    expect(warnSpy).toHaveBeenCalledTimes(1)
    expect(warnSpy).toHaveBeenCalledWith(
      '[analytix] usage event compaction failed; keeping append-only log'
    )
    const output = warnSpy.mock.calls.flat().join(' ')
    expect(output).not.toContain(privateThreadSentinel)
    expect(output).not.toContain(privatePathSentinel)
  })

  it('keeps separate provider usage baselines when compacting same-model events', async () => {
    atomicWriteFileMock.mockImplementation(async (path: string, contents: string) => {
      await writeFile(path, contents, 'utf-8')
    })
    const sessionStore = new FileSessionStore({
      dataDir,
      usageEventCompaction: {
        maxBytes: 1,
        retentionDays: 365,
        nowIso: () => '2026-06-03T00:00:00.000Z'
      }
    })
    const usage = (tokens: number): UsageSnapshot => ({
      promptTokens: tokens,
      completionTokens: 0,
      totalTokens: tokens,
      cacheHitRate: null,
      turns: tokens
    })

    await sessionStore.appendEvent('thr_usage_providers', {
      kind: 'usage',
      seq: 1,
      timestamp: '2025-06-04T00:00:00.000Z',
      threadId: 'thr_usage_providers',
      providerId: 'deepseek',
      model: 'shared-model',
      usage: usage(1)
    })
    await sessionStore.appendEvent('thr_usage_providers', {
      kind: 'usage',
      seq: 2,
      timestamp: '2025-06-04T01:00:00.000Z',
      threadId: 'thr_usage_providers',
      providerId: 'openai-compatible',
      model: 'shared-model',
      usage: usage(2)
    })
    await sessionStore.appendEvent('thr_usage_providers', {
      kind: 'usage',
      seq: 3,
      timestamp: '2025-06-04T02:00:00.000Z',
      threadId: 'thr_usage_providers',
      providerId: 'deepseek',
      model: 'shared-model',
      usage: usage(3)
    })

    const events = await sessionStore.loadEventsSince('thr_usage_providers', 0)
    expect(events.map((event) => event.seq)).toEqual([2, 3])
    expect(events.map((event) => event.kind === 'usage' ? event.providerId : '')).toEqual([
      'openai-compatible',
      'deepseek'
    ])
    expect(atomicWriteFileMock).toHaveBeenCalled()
  })

  it('keeps separate usage source and child run baselines when compacting events', async () => {
    atomicWriteFileMock.mockImplementation(async (path: string, contents: string) => {
      await writeFile(path, contents, 'utf-8')
    })
    const sessionStore = new FileSessionStore({
      dataDir,
      usageEventCompaction: {
        maxBytes: 1,
        retentionDays: 365,
        nowIso: () => '2026-06-03T00:00:00.000Z'
      }
    })
    const usage = (tokens: number): UsageSnapshot => ({
      promptTokens: tokens,
      completionTokens: 0,
      totalTokens: tokens,
      cacheHitRate: null,
      turns: tokens
    })

    await sessionStore.appendEvent('thr_usage_sources', {
      kind: 'usage',
      seq: 1,
      timestamp: '2025-06-04T00:00:00.000Z',
      threadId: 'thr_usage_sources',
      providerId: 'deepseek',
      model: 'shared-model',
      usageSource: 'executor',
      usage: usage(1)
    })
    await sessionStore.appendEvent('thr_usage_sources', {
      kind: 'usage',
      seq: 2,
      timestamp: '2025-06-04T01:00:00.000Z',
      threadId: 'thr_usage_sources',
      providerId: 'deepseek',
      model: 'shared-model',
      usageSource: 'subagent',
      childRunId: 'child_a',
      usage: usage(2)
    })
    await sessionStore.appendEvent('thr_usage_sources', {
      kind: 'usage',
      seq: 3,
      timestamp: '2025-06-04T02:00:00.000Z',
      threadId: 'thr_usage_sources',
      providerId: 'deepseek',
      model: 'shared-model',
      usageSource: 'subagent',
      childRunId: 'child_b',
      usage: usage(3)
    })
    await sessionStore.appendEvent('thr_usage_sources', {
      kind: 'usage',
      seq: 4,
      timestamp: '2025-06-04T03:00:00.000Z',
      threadId: 'thr_usage_sources',
      providerId: 'deepseek',
      model: 'shared-model',
      usageSource: 'subagent',
      childRunId: 'child_a',
      usage: usage(4)
    })

    const events = await sessionStore.loadEventsSince('thr_usage_sources', 0)
    expect(events.map((event) => event.seq)).toEqual([1, 3, 4])
    expect(events.map((event) => event.kind === 'usage' ? [
      event.usageSource,
      event.childRunId ?? ''
    ] : [])).toEqual([
      ['executor', ''],
      ['subagent', 'child_b'],
      ['subagent', 'child_a']
    ])
    expect(atomicWriteFileMock).toHaveBeenCalled()
  })
})
