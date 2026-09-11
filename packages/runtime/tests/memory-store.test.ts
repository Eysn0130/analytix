import { mkdtemp, readdir, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { buildMemoryToolProviders } from '../src/tool-test-support/tool/memory-tool-provider.js'
import { AnalytixCapabilitiesConfig, type MemoryCapabilityConfig } from '../src/contracts/capabilities.js'
import { FileMemoryStore } from '../src/memory/memory-store.js'
import type { ModelClient, ModelRequest } from '../src/ports/model-client.js'
import { dispatchRequest } from '../src/server-test-support/http-server.js'
import { bootstrapThread, makeHarness } from './loop-test-harness.js'
import { buildHarness, readJson } from './http-server-test-harness.js'

describe('Memory store and recall', () => {
  let dir = ''
  let nextId = 1

  beforeEach(async () => {
    dir = await mkdtemp(join(tmpdir(), 'analytix-memory-'))
    nextId = 1
  })

  afterEach(async () => {
    await rm(dir, { recursive: true, force: true })
  })

  it('stores manual records without retrieval and keeps tombstones', async () => {
    const store = createStore()
    const memory = await store.create({
      content: 'User prefers pnpm for frontend projects',
      scope: 'workspace',
      workspace: '/tmp/ws',
      tags: ['frontend'],
      confidence: 0.9
    })
    await store.create({
      content: 'Unrelated backend preference',
      scope: 'workspace',
      workspace: '/tmp/other'
    })

    expect(memory).toMatchObject({ provenance: 'manual-general', captureMode: 'manual', modelInjection: false })
    expect(await store.retrieve({ query: 'frontend pnpm preference', workspace: '/tmp/ws', limit: 3 })).toEqual([])
    expect(await createStore({ enabled: false }).retrieve({ query: 'pnpm', workspace: '/tmp/ws', limit: 3 })).toEqual([])

    await store.update(memory.id, { disabled: true })
    expect(await store.retrieve({ query: 'pnpm', workspace: '/tmp/ws', limit: 3 })).toEqual([])
    await store.update(memory.id, { disabled: false, content: 'User strongly prefers pnpm' })
    expect(await store.retrieve({ query: 'pnpm', workspace: '/tmp/ws', limit: 3 })).toEqual([])
    const tombstone = await store.delete(memory.id)
    expect(await store.retrieve({ query: 'pnpm', workspace: '/tmp/ws', limit: 3 })).toEqual([])
    expect(tombstone).toEqual({
      id: memory.id,
      createdAt: '2026-06-03T00:00:00.000Z',
      updatedAt: '2026-06-03T00:00:00.000Z',
      deletedAt: '2026-06-03T00:00:00.000Z'
    })
    expect(Object.keys(tombstone).sort()).toEqual(['createdAt', 'deletedAt', 'id', 'updatedAt'])
    expect(await store.list({ workspace: '/tmp/ws', includeDeleted: true })).toEqual([])
    expect((await createStore().list({ includeDeleted: true })).find((item) => item.id === memory.id)).toEqual(tombstone)
    const persisted = JSON.parse(await readFile(join(dir, 'memory', `${memory.id}.json`), 'utf8')) as Record<string, unknown>
    expect(persisted).toEqual(tombstone)
    expect(JSON.stringify(persisted)).not.toContain('pnpm')
  })

  it('exposes memory API routes with diagnostics', async () => {
    const h = buildHarness()
    h.runtime.memoryStore = createStore()
    const created = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/memory', {
        method: 'POST',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({
          content: 'Remember pnpm',
          scope: 'workspace',
          workspace: '/tmp/ws'
        })
      })
    )
    expect(created.status).toBe(201)
    const body = await readJson(created) as { memory: { id: string } }

    const list = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/memory?workspace=/tmp/ws', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect((await readJson(list)) as { memories: unknown[] }).toMatchObject({ memories: [expect.any(Object)] })

    const disabled = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/memory/${body.memory.id}`, {
        method: 'PATCH',
        headers: { authorization: 'Bearer tok-1', 'content-type': 'application/json' },
        body: JSON.stringify({ disabled: true })
      })
    )
    expect(disabled.status).toBe(200)
    const deleted = await dispatchRequest(
      h.router,
      new Request(`http://localhost/v1/memory/${body.memory.id}`, {
        method: 'DELETE',
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(deleted.status).toBe(200)
    const diagnostics = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/memory/diagnostics', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(await readJson(diagnostics)).toEqual({
      enabled: true,
      status: 'ok',
      activeCount: 0,
      tombstoneCount: 1,
    })
    expect(diagnostics.body).not.toContain(dir)
    expect(diagnostics.body).not.toContain('rootDir')
    expect(diagnostics.body).not.toContain('lastInjectedIds')
  })

  it('fails closed when public memory diagnostics cannot read the private store', async () => {
    const h = buildHarness()
    const failingStore = createStore()
    failingStore.diagnostics = async () => {
      throw new Error(`private memory path: ${dir}`)
    }
    h.runtime.memoryStore = failingStore
    const diagnostics = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/memory/diagnostics', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )

    expect(await readJson(diagnostics)).toEqual({
      enabled: true,
      status: 'unavailable',
      reasonCode: 'memory_store_read_failed'
    })
    expect(diagnostics.body).not.toContain(dir)
    expect(diagnostics.body).not.toContain('private memory path')
    expect(diagnostics.body).not.toContain('rootDir')
    expect(diagnostics.body).not.toContain('lastInjectedIds')
  })

  it('does not advertise model-callable memory mutation tools', async () => {
    const store = createStore()
    expect(buildMemoryToolProviders(store)).toEqual([])
  })

  it('never retrieves or injects Memory into AgentLoop requests', async () => {
    const store = createStore()
    const memory = await store.create({
      content: 'Use pnpm when touching frontend code',
      scope: 'workspace',
      workspace: '/tmp/ws'
    })
    let retrieveCalls = 0
    store.retrieve = async () => {
      retrieveCalls += 1
      return [memory]
    }
    const seenRequests: ModelRequest[] = []
    const model: ModelClient = {
      provider: 'fake',
      model: 'fake',
      async *stream(request) {
        seenRequests.push(request)
        yield { kind: 'completed', stopReason: 'stop' }
      }
    }
    const h = makeHarness(model, { memoryStore: store })
    await bootstrapThread(h, { workspace: '/tmp/ws', request: { prompt: 'frontend pnpm setup?' } })

    await h.loop.runTurn(h.threadId, h.turnId)

    const context = seenRequests.at(-1)?.contextInstructions?.join('\n') ?? ''
    expect(retrieveCalls).toBe(0)
    expect(context).not.toContain(memory.id)
    expect(context).not.toContain(memory.content)
    expect((await h.turns.getTurn(h.threadId, h.turnId))?.injectedMemoryIds).toEqual([])
    expect((await store.diagnostics()).lastInjectedIds).toEqual([])
  })

  it('rejects known case structures but does not claim guessed free-text PII detection', async () => {
    const store = createStore()
    for (const content of [
      `authority cer1_${'a'.repeat(64)}`,
      'continue with acct:1',
      'source /private/case.duckdb',
      '{"structuredContent":{"subjectRef":"private"}}',
      'account 6222021234567890123',
      'card 6222-0212 3456-7890',
      'malformed prefix 1--6222021234567890'
    ]) {
      await expect(store.create({ content })).rejects.toThrow('memory content is not admissible')
    }
    await expect(store.create({
      content: 'general manual preference',
      tags: ['card:1']
    })).rejects.toThrow('memory tag is not admissible')
    await expect(store.create({
      content: 'general manual preference',
      sourceThreadId: 'case-thread'
    } as never)).rejects.toThrow()
    const general = await store.create({ content: 'Manual general text may include unknown facts the owner cannot classify completely' })
    expect(general).toMatchObject({ provenance: 'manual-general', captureMode: 'manual' })
    expect(await store.retrieve({ query: general.content, limit: 1 })).toEqual([])
  })

  it('writes memory records atomically (no .tmp file left on success)', async () => {
    const store = createStore()
    await store.create({ content: 'atomic test memory' })

    // Final file present and parseable.
    const finalContents = await readFile(
      join(dir, 'memory', 'mem_1.json'),
      'utf8'
    )
    expect(finalContents.length).toBeGreaterThan(0)
    expect(JSON.parse(finalContents).content).toBe('atomic test memory')

    // No .tmp leftover from the atomic write.
    const entries = await readdir(join(dir, 'memory'))
    expect(entries.filter((entry) => entry.includes('.tmp'))).toEqual([])
  })

  function createStore(overrides: Partial<MemoryCapabilityConfig> = {}) {
    return new FileMemoryStore({
      rootDir: join(dir, 'memory'),
      config: memoryConfig(overrides),
      nowIso: () => '2026-06-03T00:00:00.000Z',
      idGenerator: () => `mem_${nextId++}`
    })
  }

  function memoryConfig(overrides: Partial<MemoryCapabilityConfig> = {}) {
    return AnalytixCapabilitiesConfig.parse({
      memory: {
        enabled: true,
        ...overrides
      }
    }).memory
  }
})
