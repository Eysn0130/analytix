import { mkdtemp, mkdir, readFile, rm, stat, writeFile } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { InMemoryEventBus } from '../src/adapters/in-memory-event-bus.js'
import { HybridSessionStore, HybridThreadStore } from '../src/adapters/hybrid/index.js'
import { makeAssistantTextItem, makeUserItem } from '../src/domain/item.js'
import { appendTurnItem, createTurnRecord, startTurn } from '../src/domain/turn.js'
import { createThreadRecord } from '../src/domain/thread.js'
import { RuntimeEventRecorder } from '../src/services-test-support/runtime-event-recorder.js'
import { ThreadService } from '../src/services-test-support/thread-service.js'
import { TurnService } from '../src/services-test-support/turn-service.js'
import { InflightTracker } from '../src/loop-test-support/inflight-tracker.js'
import { SteeringQueue } from '../src/loop-test-support/steering-queue.js'
import { ContextCompactor } from '../src/shared/context-compactor.js'
import { SequentialIdGenerator } from '../src/ports/id-generator.js'
import type { UsageSnapshot } from '../src/contracts/usage.js'

describe('HybridThreadStore', () => {
  let dataDir = ''
  let openStores: HybridThreadStore[] = []
  let sqliteAvailable = false
  let previousDisableSqlite: string | undefined

  beforeEach(async () => {
    previousDisableSqlite = process.env.ANALYTIX_RUNTIME_DISABLE_SQLITE
    dataDir = await mkdtemp(join(tmpdir(), 'analytix-hybrid-'))
    openStores = []
    sqliteAvailable = await canOpenBetterSqlite()
    if (!sqliteAvailable) {
      process.env.ANALYTIX_RUNTIME_DISABLE_SQLITE = '1'
    }
  })

  afterEach(async () => {
    for (const store of openStores) store.close()
    await rm(dataDir, { recursive: true, force: true })
    if (previousDisableSqlite === undefined) {
      delete process.env.ANALYTIX_RUNTIME_DISABLE_SQLITE
    } else {
      process.env.ANALYTIX_RUNTIME_DISABLE_SQLITE = previousDisableSqlite
    }
  })

  it('keeps item bodies in JSONL and uses SQLite metadata indexing when available', async () => {
    const { threadStore, sessionStore } = await createHybridStores()
    const record = await seedThreadWithMessage(threadStore, sessionStore, 'hello from jsonl')

    const summaries = await threadStore.list({ search: 'Hybrid demo' })
    expect(summaries.map((thread) => thread.id)).toEqual([record.id])
    if (sqliteAvailable) {
      await expect(stat(join(dataDir, 'index.sqlite3'))).resolves.toBeTruthy()
    } else {
      await expect(stat(join(dataDir, 'index.sqlite3'))).rejects.toMatchObject({ code: 'ENOENT' })
    }

    const metadata = await readFile(
      join(dataDir, 'threads', record.id, 'metadata.jsonl'),
      'utf-8'
    )
    const messages = await readFile(
      join(dataDir, 'threads', record.id, 'messages.jsonl'),
      'utf-8'
    )
    expect(metadata).toContain('"preview":"hello from jsonl"')
    expect(metadata).toContain('"turnCount":1')
    expect(metadata).toContain('"messageCount":1')
    expect(messages).toContain('hello from jsonl')

    const fetched = await threadStore.get(record.id)
    expect(fetched?.turns[0]?.prompt).toBe('hello from jsonl')
    expect(fetched?.turns[0]?.items[0]).toMatchObject({
      kind: 'user_message',
      text: 'hello from jsonl'
    })
  })

  it('projects SQLite fallback warnings without raw paths or error text', async () => {
    const privatePathSentinel = 'SLICE48_SQLITE_PRIVATE_PATH_SENTINEL'
    const fallbackDataDir = join(dataDir, 'fallback-data')
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    delete process.env.ANALYTIX_RUNTIME_DISABLE_SQLITE
    vi.doMock('better-sqlite3', () => ({
      default: class FailingDatabase {
        constructor() {
          throw new Error(`/private/${privatePathSentinel}/index.sqlite3`)
        }
      }
    }))
    try {
      const threadStore = new HybridThreadStore({ dataDir: fallbackDataDir })
      openStores.push(threadStore)

      await threadStore.ready()

      const output = warn.mock.calls.flat().join(' ')
      expect(output).toBe('[analytix] event=ANALYTIX_HYBRID_SQLITE_FALLBACK')
      expect(output).not.toContain(privatePathSentinel)
    } finally {
      warn.mockRestore()
      vi.doUnmock('better-sqlite3')
      vi.resetModules()
    }
  })

  it('projects metadata compaction warnings without thread ids or error paths', async () => {
    const privateThreadSentinel = 'thr_SLICE48_PRIVATE_THREAD_SENTINEL'
    const compactionDataDir = join(dataDir, 'compaction-data')
    process.env.ANALYTIX_RUNTIME_DISABLE_SQLITE = '1'
    const threadStore = new HybridThreadStore({ dataDir: compactionDataDir })
    openStores.push(threadStore)
    await threadStore.ready()
    const thread = createThreadRecord({
      id: privateThreadSentinel,
      title: 'x'.repeat(1_100_000),
      workspace: `/private/${privateThreadSentinel}`,
      model: 'deepseek-chat',
      createdAt: '2026-08-26T00:00:00.000Z'
    })
    await mkdir(join(
      compactionDataDir,
      'threads',
      privateThreadSentinel,
      'metadata.jsonl.compact.tmp'
    ), { recursive: true })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)

    try {
      await threadStore.upsert(thread)

      const output = warn.mock.calls.flat().join(' ')
      expect(output).toBe('[analytix] event=ANALYTIX_HYBRID_METADATA_COMPACTION_SKIPPED')
      expect(output).not.toContain(privateThreadSentinel)
    } finally {
      warn.mockRestore()
    }
  })

  it('returns providerId from the SQLite list index', async () => {
    if (!sqliteAvailable) return
    const { threadStore, sessionStore } = await createHybridStores()
    const record = await seedThreadWithMessage(
      threadStore,
      sessionStore,
      'provider routed preview',
      { providerId: 'zai-coding-plan' }
    )

    const summaries = await threadStore.list({ search: 'Hybrid demo' })

    expect(summaries).toHaveLength(1)
    expect(summaries[0]).toMatchObject({
      id: record.id,
      providerId: 'zai-coding-plan'
    })
  })

  it('lists from sidecar summaries when SQLite is rebuilt and messages are unreadable', async () => {
    const first = await createHybridStores()
    const record = await seedThreadWithMessage(
      first.threadStore,
      first.sessionStore,
      'cached preview source',
      { providerId: 'zai-coding-plan' }
    )
    first.threadStore.close()

    await deleteSqliteIndex()
    await writeFile(join(dataDir, 'threads', record.id, 'messages.jsonl'), '{not-json\n', 'utf8')

    const reopened = await createHybridStores()
    await reopened.threadStore.waitForBackfill()
    const summaries = await reopened.threadStore.list({ search: 'Hybrid demo' })

    expect(summaries).toHaveLength(1)
    expect(summaries[0]).toMatchObject({
      id: record.id,
      providerId: 'zai-coding-plan',
      preview: 'cached preview source',
      turnCount: 1,
      messageCount: 1
    })
  })

  it('backfills legacy metadata summary without changing thread activity time', async () => {
    const { threadStore, sessionStore } = await createHybridStores()
    const thread = createThreadRecord({
      id: 'thr_legacy_summary',
      title: 'Legacy summary',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      createdAt: '2026-06-04T00:00:00.000Z'
    })
    const turn = createTurnRecord({
      id: 'turn_legacy_summary',
      threadId: thread.id,
      prompt: 'legacy sidecar question',
      model: thread.model,
      createdAt: '2026-06-04T00:00:01.000Z'
    })
    const item = makeUserItem({
      id: 'item_turn_legacy_summary_user',
      turnId: turn.id,
      threadId: thread.id,
      text: 'legacy sidecar question'
    })
    const record = {
      ...thread,
      updatedAt: '2026-06-04T00:00:02.000Z',
      turns: [startTurn(appendTurnItem(turn, item), '2026-06-04T00:00:01.000Z')]
    }
    await sessionStore.appendItem(record.id, item)
    await threadStore.upsert(record)
    threadStore.close()

    const metadataPath = join(dataDir, 'threads', record.id, 'metadata.jsonl')
    const legacyLine = {
      kind: 'thread_metadata',
      version: 1,
      timestamp: '2026-06-04T00:00:03.000Z',
      thread: { ...record, turns: record.turns.map((recordTurn) => ({ ...recordTurn, prompt: '', items: [] })) }
    }
    await writeFile(metadataPath, `${JSON.stringify(legacyLine)}\n`, 'utf8')
    await deleteSqliteIndex()

    const reopened = await createHybridStores()
    await reopened.threadStore.waitForBackfill()
    const summaries = await reopened.threadStore.list({ search: 'legacy' })

    expect(summaries).toHaveLength(1)
    expect(summaries[0]).toMatchObject({
      id: record.id,
      preview: 'legacy sidecar question',
      turnCount: 1,
      messageCount: 1,
      updatedAt: '2026-06-04T00:00:02.000Z'
    })
    const metadata = await readFile(metadataPath, 'utf8')
    expect(metadata).toContain('"preview":"legacy sidecar question"')
    expect(metadata).toContain('"schemaVersion":1')
    expect(metadata).toContain('"turnCount":1')
    expect(metadata).toContain('"updatedAt":"2026-06-04T00:00:02.000Z"')
  })

  it('trusts authoritative zero-count sidecar summaries without decoding messages', async () => {
    const thread = createThreadRecord({
      id: 'thr_authoritative_empty',
      title: 'Authoritative empty',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      createdAt: '2026-06-04T00:00:00.000Z'
    })
    const turn = createTurnRecord({
      id: 'turn_authoritative_empty',
      threadId: thread.id,
      prompt: 'should not be read',
      model: thread.model,
      createdAt: '2026-06-04T00:00:01.000Z'
    })
    const record = {
      ...thread,
      updatedAt: '2026-06-04T00:00:02.000Z',
      turns: [startTurn(turn, '2026-06-04T00:00:01.000Z')]
    }
    const threadDir = join(dataDir, 'threads', record.id)
    const metadataPath = join(threadDir, 'metadata.jsonl')
    await mkdir(threadDir, { recursive: true })
    await writeFile(
      metadataPath,
      `${JSON.stringify({
        kind: 'thread_metadata',
        version: 1,
        timestamp: '2026-06-04T00:00:03.000Z',
        thread: { ...record, turns: record.turns.map((recordTurn) => ({ ...recordTurn, prompt: '', items: [] })) },
        summary: { schemaVersion: 1, preview: '', messageCount: 0, turnCount: 0 }
      })}\n`,
      'utf8'
    )
    await writeFile(
      join(threadDir, 'messages.jsonl'),
      `${JSON.stringify(makeUserItem({
        id: 'item_authoritative_empty_user',
        turnId: turn.id,
        threadId: thread.id,
        text: 'content that would prove the messages file was decoded'
      }))}\n`,
      'utf8'
    )

    const reopened = await createHybridStores()
    await reopened.threadStore.waitForBackfill()
    const summaries = await reopened.threadStore.list({ search: 'Authoritative empty' })

    expect(summaries).toHaveLength(1)
    expect(summaries[0]).toMatchObject({
      id: record.id,
      messageCount: 0,
      turnCount: 0,
      updatedAt: '2026-06-04T00:00:02.000Z'
    })
    expect(summaries[0]?.preview).toBeUndefined()
    const metadata = await readFile(metadataPath, 'utf8')
    expect(metadata.trim().split('\n')).toHaveLength(1)
    expect(metadata).not.toContain('content that would prove')
  })

  it('decodes legacy sidecar summaries once, stamps them, and preserves updatedAt', async () => {
    const { threadStore, sessionStore } = await createHybridStores()
    const thread = createThreadRecord({
      id: 'thr_legacy_summary_version',
      title: 'Legacy version summary',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      createdAt: '2026-06-04T00:00:00.000Z'
    })
    const turn = createTurnRecord({
      id: 'turn_legacy_summary_version',
      threadId: thread.id,
      prompt: 'legacy version prompt',
      model: thread.model,
      createdAt: '2026-06-04T00:00:01.000Z'
    })
    const item = makeUserItem({
      id: 'item_legacy_summary_version_user',
      turnId: turn.id,
      threadId: thread.id,
      text: 'legacy version prompt'
    })
    const record = {
      ...thread,
      updatedAt: '2026-06-04T00:00:02.000Z',
      turns: [startTurn(appendTurnItem(turn, item), '2026-06-04T00:00:01.000Z')]
    }
    await sessionStore.appendItem(record.id, item)
    await threadStore.upsert(record)
    threadStore.close()

    const threadDir = join(dataDir, 'threads', record.id)
    const metadataPath = join(threadDir, 'metadata.jsonl')
    const legacyLine = {
      kind: 'thread_metadata',
      version: 1,
      timestamp: '2026-06-04T00:00:03.000Z',
      thread: { ...record, turns: record.turns.map((recordTurn) => ({ ...recordTurn, prompt: '', items: [] })) },
      summary: { preview: '', messageCount: 0, turnCount: 0 }
    }
    await writeFile(metadataPath, `${JSON.stringify(legacyLine)}\n`, 'utf8')
    await deleteSqliteIndex()

    const reopened = await createHybridStores()
    await reopened.threadStore.waitForBackfill()
    const summaries = await reopened.threadStore.list({ search: 'legacy version' })

    expect(summaries).toHaveLength(1)
    expect(summaries[0]).toMatchObject({
      id: record.id,
      preview: 'legacy version prompt',
      messageCount: 1,
      turnCount: 1,
      updatedAt: '2026-06-04T00:00:02.000Z'
    })
    let metadata = await readFile(metadataPath, 'utf8')
    let lines = metadata.trim().split('\n')
    expect(lines).toHaveLength(2)
    let stamped = JSON.parse(lines[1]) as { thread: { updatedAt: string }; summary: { schemaVersion?: number } }
    expect(stamped.summary.schemaVersion).toBe(1)
    expect(stamped.thread.updatedAt).toBe('2026-06-04T00:00:02.000Z')

    await writeFile(
      join(threadDir, 'messages.jsonl'),
      `${JSON.stringify(makeUserItem({
        id: 'item_legacy_summary_version_user_changed',
        turnId: turn.id,
        threadId: thread.id,
        text: 'changed after summary stamp'
      }))}\n`,
      'utf8'
    )
    reopened.threadStore.close()
    await deleteSqliteIndex()

    const secondReopen = await createHybridStores()
    await secondReopen.threadStore.waitForBackfill()
    const secondSummaries = await secondReopen.threadStore.list({ search: 'legacy version' })

    expect(secondSummaries).toHaveLength(1)
    expect(secondSummaries[0]).toMatchObject({
      preview: 'legacy version prompt',
      messageCount: 1,
      turnCount: 1
    })
    metadata = await readFile(metadataPath, 'utf8')
    lines = metadata.trim().split('\n')
    expect(lines).toHaveLength(2)
    expect(metadata).not.toContain('changed after summary stamp')
  })

  it('lists existing SQLite rows without replaying damaged message or event logs', async () => {
    const first = await createHybridStores()
    const record = await seedThreadWithMessage(first.threadStore, first.sessionStore, 'indexed already')
    first.threadStore.close()

    await writeFile(join(dataDir, 'threads', record.id, 'messages.jsonl'), '{not-json\n', 'utf8')
    await writeFile(join(dataDir, 'threads', record.id, 'events.jsonl'), '{not-json\n', 'utf8')

    const reopened = await createHybridStores()
    const summaries = await reopened.threadStore.list({ search: 'Hybrid demo' })

    expect(summaries.map((thread) => thread.id)).toEqual([record.id])
  })

  it('rebuilds the SQLite index from JSONL after the database is deleted', async () => {
    const first = await createHybridStores()
    const record = await seedThreadWithMessage(first.threadStore, first.sessionStore, 'recover me')
    first.threadStore.close()

    await rm(join(dataDir, 'index.sqlite3'), { force: true })
    await rm(join(dataDir, 'index.sqlite3-wal'), { force: true })
    await rm(join(dataDir, 'index.sqlite3-shm'), { force: true })

    const rebuilt = await createHybridStores()
    await rebuilt.threadStore.waitForBackfill()
    const summaries = await rebuilt.threadStore.list({ search: 'Hybrid demo' })
    expect(summaries.map((thread) => thread.id)).toEqual([record.id])

    const fetched = await rebuilt.threadStore.get(record.id)
    expect(fetched?.turns[0]?.items[0]).toMatchObject({
      kind: 'user_message',
      text: 'recover me'
    })
  })

  it('seeds fork sidecar summaries at creation', async () => {
    const { threadStore, sessionStore } = await createHybridStores()
    await seedThreadWithTurns(threadStore, sessionStore, {
      id: 'thr_parent',
      title: 'Parent thread',
      prompts: ['root prompt', 'follow up']
    })
    const threads = createThreadService(threadStore, sessionStore)

    const fork = await threads.fork('thr_parent', { relation: 'fork', title: 'Forked copy' })
    const forkSummary = (await threadStore.list({ search: 'Forked copy' })).find((thread) => thread.id === fork.id)

    expect(forkSummary).toMatchObject({
      id: fork.id,
      preview: 'root prompt',
      turnCount: 2,
      messageCount: 4,
      forkedFromThreadId: 'thr_parent',
      forkedFromTurnCount: 2
    })

    const side = await threads.fork('thr_parent', { relation: 'side', title: 'Side copy' })
    const sideSummary = (await threadStore.list({ search: 'Side copy', includeSide: true }))
      .find((thread) => thread.id === side.id)

    expect(sideSummary).toMatchObject({
      id: side.id,
      preview: 'root prompt',
      turnCount: 2,
      messageCount: 2,
      relation: 'side',
      forkedFromThreadId: 'thr_parent',
      forkedFromTurnCount: 2
    })

    threadStore.close()
    await deleteSqliteIndex()
    await writeFile(join(dataDir, 'threads', fork.id, 'messages.jsonl'), '{not-json\n', 'utf8')

    const reopened = await createHybridStores()
    await reopened.threadStore.waitForBackfill()
    const rebuiltFork = (await reopened.threadStore.list({ search: 'Forked copy' }))
      .find((thread) => thread.id === fork.id)

    expect(rebuiltFork).toMatchObject({
      preview: 'root prompt',
      turnCount: 2,
      messageCount: 4
    })
  })

  it('indexes event high water and usage events as they are appended', async () => {
    if (!sqliteAvailable) return
    const { threadStore, sessionStore } = await createHybridStores()
    const record = await seedThreadWithMessage(threadStore, sessionStore, 'track usage')
    await sessionStore.appendEvent(record.id, {
      kind: 'usage',
      seq: 2,
      timestamp: '2026-06-04T00:00:03.000Z',
      threadId: record.id,
      turnId: 'turn_hybrid',
      model: 'deepseek-chat',
      usage: usage({ promptTokens: 10, completionTokens: 5, totalTokens: 15, turns: 1 })
    })
    await sessionStore.appendEvent(record.id, {
      kind: 'usage',
      seq: 5,
      timestamp: '2026-06-04T00:00:05.000Z',
      threadId: record.id,
      turnId: 'turn_hybrid',
      model: 'deepseek-chat',
      usage: usage({
        promptTokens: 30,
        completionTokens: 10,
        totalTokens: 40,
        turns: 2,
        cacheHitTokens: 20,
        cacheMissTokens: 10,
        cacheableTokenHitRate: 20 / 30,
        totalInputTokenHitRate: 20 / 30,
        cacheMissReasons: ['stable_prefix_changed'],
        cacheSuggestions: ['Keep volatile workspace context out of the stable prefix.']
      })
    })

    await writeFile(join(dataDir, 'threads', record.id, 'events.jsonl'), '{not-json\n', 'utf8')

    await expect(sessionStore.highestSeq(record.id)).resolves.toBe(5)
    await expect(sessionStore.loadLatestUsageSnapshots()).resolves.toMatchObject([
      {
        threadId: record.id,
        seq: 5,
        usage: {
          promptTokens: 30,
          completionTokens: 10,
          totalTokens: 40,
          turns: 2
        }
      }
    ])
    await expect(sessionStore.loadUsageRecords({ threadId: record.id })).resolves.toMatchObject([
      {
        threadId: record.id,
        model: 'deepseek-chat',
        completedAt: '2026-06-04T00:00:03.000Z',
        usage: { totalTokens: 15, turns: 1 }
      },
      {
        threadId: record.id,
        model: 'deepseek-chat',
        completedAt: '2026-06-04T00:00:05.000Z',
        usage: {
          totalTokens: 25,
          turns: 1,
          cacheableTokenHitRate: 1,
          totalInputTokenHitRate: 20 / 20,
          cacheMissReasons: ['stable_prefix_changed'],
          cacheSuggestions: ['Keep volatile workspace context out of the stable prefix.']
        }
      }
    ])
  })

  it('recovers turn attachment ids from user messages when metadata is stripped', async () => {
    const { threadStore, sessionStore } = await createHybridStores()
    const thread = createThreadRecord({
      id: 'thr_attach',
      title: 'Attachment demo',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      createdAt: '2026-06-04T00:00:00.000Z'
    })
    const turn = createTurnRecord({
      id: 'turn_attach',
      threadId: thread.id,
      prompt: 'describe',
      model: thread.model,
      createdAt: '2026-06-04T00:00:01.000Z'
    })
    const item = makeUserItem({
      id: 'item_turn_attach_user',
      turnId: turn.id,
      threadId: thread.id,
      text: 'describe',
      attachmentIds: ['att_image']
    })
    const record = {
      ...thread,
      updatedAt: '2026-06-04T00:00:02.000Z',
      turns: [startTurn(appendTurnItem(turn, item), '2026-06-04T00:00:01.000Z')]
    }
    await sessionStore.appendItem(record.id, item)
    await threadStore.upsert(record)

    const fetched = await threadStore.get(record.id)

    expect(fetched?.turns[0]?.attachmentIds).toEqual(['att_image'])
    expect(fetched?.turns[0]?.items[0]).toMatchObject({
      kind: 'user_message',
      attachmentIds: ['att_image']
    })
  })

  it('does not synthesize duplicate turns when startTurn writes through the hybrid store', async () => {
    const { threadStore, sessionStore } = await createHybridStores()
    const thread = createThreadRecord({
      id: 'thr_start',
      title: 'Start demo',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      createdAt: '2026-06-04T00:00:00.000Z'
    })
    await threadStore.upsert(thread)
    const turns = createTurnService(threadStore, sessionStore)

    const response = await turns.startTurn({
      threadId: thread.id,
      request: {
        prompt: 'describe this data',
        model: 'deepseek-v4-pro',
        providerId: 'zai-coding-plan',
        attachmentIds: ['att_image'],
        fileReferences: [{
          path: '/tmp/project/src/data.csv',
          relativePath: 'src/data.csv',
          name: 'data.csv',
          kind: 'file'
        }],
        mode: 'agent'
      }
    })
    const fetched = await threadStore.get(thread.id)
    const items = await sessionStore.loadItems(thread.id)

    expect(fetched?.turns.map((turn) => turn.id)).toEqual([response.turnId])
    expect(fetched?.providerId).toBe('zai-coding-plan')
    expect(fetched?.turns[0]).toMatchObject({
      id: response.turnId,
      attachmentIds: ['att_image'],
      model: 'deepseek-v4-pro'
    })
    expect(fetched?.turns[0]?.items[0]).toMatchObject({
      kind: 'user_message',
      attachmentIds: ['att_image'],
      fileReferences: [{ relativePath: 'src/data.csv', kind: 'file' }]
    })
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({
      kind: 'user_message',
      attachmentIds: ['att_image'],
      fileReferences: [{ relativePath: 'src/data.csv', kind: 'file' }]
    })
  })

  it('deduplicates damaged turn metadata and recovers attachment ids from earlier metadata lines', async () => {
    const { threadStore, sessionStore } = await createHybridStores()
    const thread = createThreadRecord({
      id: 'thr_damaged',
      title: 'Damaged metadata',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      createdAt: '2026-06-04T00:00:00.000Z'
    })
    const turn = startTurn(
      createTurnRecord({
        id: 'turn_damaged',
        threadId: thread.id,
        prompt: 'describe',
        model: 'deepseek-v4-pro',
        attachmentIds: ['att_from_history'],
        createdAt: '2026-06-04T00:00:01.000Z'
      }),
      '2026-06-04T00:00:01.500Z'
    )
    const damagedTurn = {
      ...turn,
      status: 'completed' as const,
      prompt: '',
      items: [],
      attachmentIds: [],
      finishedAt: '2026-06-04T00:00:03.000Z'
    }
    await mkdir(join(dataDir, 'threads', thread.id), { recursive: true })
    await writeFile(
      join(dataDir, 'threads', thread.id, 'metadata.jsonl'),
      [
        {
          kind: 'thread_metadata',
          version: 1,
          timestamp: '2026-06-04T00:00:02.000Z',
          thread: { ...thread, status: 'running', turns: [{ ...turn, prompt: '', items: [] }] }
        },
        {
          kind: 'thread_metadata',
          version: 1,
          timestamp: '2026-06-04T00:00:03.000Z',
          thread: {
            ...thread,
            status: 'idle',
            updatedAt: '2026-06-04T00:00:03.000Z',
            turns: [damagedTurn, damagedTurn]
          }
        }
      ].map((line) => JSON.stringify(line)).join('\n') + '\n',
      'utf8'
    )
    await sessionStore.appendItem(thread.id, makeUserItem({
      id: 'item_turn_damaged_user',
      turnId: turn.id,
      threadId: thread.id,
      text: 'describe'
    }))

    const fetched = await threadStore.get(thread.id)

    expect(fetched?.turns).toHaveLength(1)
    expect(fetched?.turns[0]).toMatchObject({
      id: turn.id,
      attachmentIds: ['att_from_history']
    })
    expect(fetched?.turns[0]?.items[0]).toMatchObject({
      kind: 'user_message',
      text: 'describe'
    })
  })

  async function createHybridStores(): Promise<{
    threadStore: HybridThreadStore
    sessionStore: HybridSessionStore
  }> {
    const threadStore = new HybridThreadStore({ dataDir })
    await threadStore.ready()
    openStores.push(threadStore)
    return {
      threadStore,
      sessionStore: new HybridSessionStore({ dataDir, index: threadStore })
    }
  }

  async function seedThreadWithMessage(
    threadStore: HybridThreadStore,
    sessionStore: HybridSessionStore,
    text: string,
    options: { providerId?: string } = {}
  ) {
    const thread = createThreadRecord({
      id: 'thr_hybrid',
      title: 'Hybrid demo',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      providerId: options.providerId,
      createdAt: '2026-06-04T00:00:00.000Z'
    })
    const turn = createTurnRecord({
      id: 'turn_hybrid',
      threadId: thread.id,
      prompt: text,
      model: thread.model,
      createdAt: '2026-06-04T00:00:01.000Z'
    })
    const item = makeUserItem({
      id: 'item_turn_hybrid_user',
      turnId: turn.id,
      threadId: thread.id,
      text
    })
    const record = {
      ...thread,
      updatedAt: '2026-06-04T00:00:02.000Z',
      turns: [startTurn(appendTurnItem(turn, item), '2026-06-04T00:00:01.000Z')]
    }
    await sessionStore.appendItem(record.id, item)
    await threadStore.upsert(record)
    return record
  }

  function createTurnService(
    threadStore: HybridThreadStore,
    sessionStore: HybridSessionStore
  ): TurnService {
    const bus = new InMemoryEventBus()
    const events = new RuntimeEventRecorder({
      eventBus: bus,
      sessionStore,
      allocateSeq: (threadId) => bus.allocateSeq(threadId),
      nowIso: () => '2026-06-04T00:00:02.000Z'
    })
    return new TurnService({
      threadStore,
      sessionStore,
      events,
      inflight: new InflightTracker(),
      steering: new SteeringQueue(),
      compactor: new ContextCompactor(),
      ids: new SequentialIdGenerator(),
      nowIso: () => '2026-06-04T00:00:02.000Z'
    })
  }

  function createThreadService(
    threadStore: HybridThreadStore,
    sessionStore: HybridSessionStore
  ): ThreadService {
    const bus = new InMemoryEventBus()
    const events = new RuntimeEventRecorder({
      eventBus: bus,
      sessionStore,
      allocateSeq: (threadId) => bus.allocateSeq(threadId),
      nowIso: () => '2026-06-04T00:00:05.000Z'
    })
    return new ThreadService({
      threadStore,
      sessionStore,
      events,
      ids: new SequentialIdGenerator(),
      nowIso: () => '2026-06-04T00:00:05.000Z'
    })
  }

  async function seedThreadWithTurns(
    threadStore: HybridThreadStore,
    sessionStore: HybridSessionStore,
    options: { id: string; title: string; prompts: string[] }
  ) {
    const thread = createThreadRecord({
      id: options.id,
      title: options.title,
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      createdAt: '2026-06-04T00:00:00.000Z'
    })
    const turns = []
    for (const [index, prompt] of options.prompts.entries()) {
      const ordinal = index + 1
      const turn = createTurnRecord({
        id: `turn_${ordinal}`,
        threadId: thread.id,
        prompt,
        model: thread.model,
        createdAt: `2026-06-04T00:00:0${ordinal}.000Z`
      })
      const user = makeUserItem({
        id: `item_turn_${ordinal}_user`,
        turnId: turn.id,
        threadId: thread.id,
        text: prompt
      })
      const assistant = makeAssistantTextItem({
        id: `item_turn_${ordinal}_assistant`,
        turnId: turn.id,
        threadId: thread.id,
        text: `answer ${ordinal}`
      })
      await sessionStore.appendItem(thread.id, user)
      await sessionStore.appendItem(thread.id, assistant)
      turns.push(startTurn(
        appendTurnItem(appendTurnItem(turn, user), assistant),
        `2026-06-04T00:00:1${ordinal}.000Z`
      ))
    }
    const record = {
      ...thread,
      updatedAt: '2026-06-04T00:00:04.000Z',
      turns
    }
    await threadStore.upsert(record)
    return record
  }

  async function deleteSqliteIndex(): Promise<void> {
    await rm(join(dataDir, 'index.sqlite3'), { force: true })
    await rm(join(dataDir, 'index.sqlite3-wal'), { force: true })
    await rm(join(dataDir, 'index.sqlite3-shm'), { force: true })
  }

  async function canOpenBetterSqlite(): Promise<boolean> {
    if (process.env.ANALYTIX_RUNTIME_DISABLE_SQLITE === '1') return false
    const probe = spawnSync(process.execPath, [
      '-e',
      "import('better-sqlite3').then(({default: Database})=>{const db=new Database(':memory:'); db.close(); process.exit(0)}).catch(()=>process.exit(1))"
    ], {
      cwd: process.cwd(),
      stdio: 'ignore',
      timeout: 5_000
    })
    return probe.status === 0
  }

  function usage(overrides: Partial<UsageSnapshot>): UsageSnapshot {
    const promptTokens = overrides.promptTokens ?? 10
    const completionTokens = overrides.completionTokens ?? 5
    const cacheHitTokens = overrides.cacheHitTokens ?? 0
    const cacheMissTokens = overrides.cacheMissTokens ?? Math.max(promptTokens - cacheHitTokens, 0)
    const cacheTotal = cacheHitTokens + cacheMissTokens
    const snapshot: UsageSnapshot = {
      promptTokens,
      completionTokens,
      totalTokens: overrides.totalTokens ?? promptTokens + completionTokens,
      cachedTokens: overrides.cachedTokens ?? cacheHitTokens,
      cacheHitTokens,
      cacheMissTokens,
      cacheHitRate: cacheTotal === 0 ? null : cacheHitTokens / cacheTotal,
      turns: overrides.turns ?? 1
    }
    if (overrides.cacheableTokenHitRate !== undefined) {
      snapshot.cacheableTokenHitRate = overrides.cacheableTokenHitRate
    }
    if (overrides.totalInputTokenHitRate !== undefined) {
      snapshot.totalInputTokenHitRate = overrides.totalInputTokenHitRate
    }
    if (overrides.cacheMissReasons !== undefined) {
      snapshot.cacheMissReasons = [...overrides.cacheMissReasons]
    }
    if (overrides.cacheSuggestions !== undefined) {
      snapshot.cacheSuggestions = [...overrides.cacheSuggestions]
    }
    return snapshot
  }
})
