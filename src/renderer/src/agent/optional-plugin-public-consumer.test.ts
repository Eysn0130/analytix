import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { PublicRuntimeEventFilter } from '../../../shared/public-runtime-content'
import { projectPublicRuntimeSseBlock, takePublicRuntimeSseBlock } from '../../../shared/public-runtime-sse'
import type { AnalytixApi } from '../../../shared/analytix-api'
import { AnalytixRuntimeProvider } from './analytix-runtime'
import { rendererRuntimeClient } from './runtime-client'
import { buildThreadEventSink } from '../store/chat-store-runtime'
import type { ChatState, ChatStoreSet } from '../store/chat-store-types'
import { clearActiveStream } from '../thread/streaming/active-stream-store'
import { resetPublicProjectionRevocationsForTests } from '../lib/public-projection-revocation'
import { clearBusyWatchdog } from '../store/chat-store-schedulers'
import type { ThreadEventSink } from './types'

const runtimeProcess = (globalThis as unknown as {
  process: { env: Record<string, string | undefined>; version: string; cwd: () => string }
}).process

const electron = vi.hoisted(() => ({
  exposed: new Map<string, unknown>(),
  listeners: new Map<string, Set<(...args: unknown[]) => void>>(),
  invoke: vi.fn(),
  send: vi.fn()
}))

// Only the Electron/transport shell is replaced. Public parsers, authority
// verification, preload facade, renderer adapter and store are production code.
vi.mock('electron', () => ({
  app: { isPackaged: false, getAppPath: () => runtimeProcess.cwd(), getPath: () => '/synthetic-shell', getVersion: () => '0.0.0-test' },
  contextBridge: { exposeInMainWorld: (name: string, api: unknown) => electron.exposed.set(name, api) },
  ipcRenderer: {
    invoke: electron.invoke, send: electron.send,
    on: (channel: string, handler: (...args: unknown[]) => void) => {
      const listeners = electron.listeners.get(channel) ?? new Set()
      listeners.add(handler)
      electron.listeners.set(channel, listeners)
    },
    removeListener: (channel: string, handler: (...args: unknown[]) => void) => electron.listeners.get(channel)?.delete(handler)
  },
  webUtils: { getPathForFile: () => '' }
}))

type PublicRow = { ordinal: number; phase: string; method: string; path: string; status: number; body: string; sha256: string }
type PublicEvent = Record<string, unknown> & { kind: string; threadId: string; turnId: string; firstSeq: number; lastSeq: number; seq: number }
type Corpus = {
  schemaVersion: number
  fault: string
  input: { commandId: string; head: string; files: Record<string, string>; nodeVersion: string; goVersion: string; scheduleBundle: string }
  responses: PublicRow[]
  operations: Array<{ ordinal: number; phase: string; threadId: string; turnId: string; starts: number; ends: number; completionErrorClass: string; failureRecordErrorClass: string; terminalStatus: string }>
  publicLauncherAuthority: { keyId: string; publicKey: string }
  privacyCheckedBeforeSampling: boolean
  drained: boolean
}

const faults = ['missing', 'disabled', 'incompatible', 'unauthorized', 'domain-semantic']
const activation = runtimeProcess.env.ANALYTIX_TEST_SHARED_PUBLIC_CONSUMER_V1 ?? ''
const corpusRoot = runtimeProcess.env.ANALYTIX_TEST_SHARED_PUBLIC_CORPUS_V1 ?? ''
const corpora = new Map<string, Corpus>()
let digest: (body: string | Uint8Array) => string
let readFileSync: { (path: string): Uint8Array; (path: string, encoding: 'utf8'): string }
let readdirSync: (path: string) => string[]
let join: (...paths: string[]) => string
let api: AnalytixApi
// Load the OS-side module graph at test runtime. Pulling its ambient Node timer
// declarations into the renderer compilation changes unrelated DOM overloads.
let sanitizeRuntimeResponse: (
  response: { ok: boolean; status: number; body: string }, path: string,
  pin: Corpus['publicLauncherAuthority'], method: string
) => { ok: boolean; status: number; body: string }
let verifiedAcceptedFinalDeliveryBatch: (event: Record<string, unknown>, pin: Corpus['publicLauncherAuthority']) => { batch: Record<string, unknown> } | null
let generalTerminalDeliveryBatchVerificationV1: (event: Record<string, unknown>) => { reason: string | null; verified: { batch: Record<string, unknown> } | null }
let responseRow: PublicRow | null = null
let currentCorpus: Corpus
let eventsToDeliver: Record<string, unknown>[] = []
let beforeAck: ((seq: number, binding: unknown) => void) | undefined
let deliveredStream = ''
let ackCount = 0

function emit(channel: string, payload: unknown): void {
  for (const handler of electron.listeners.get(channel) ?? []) handler({}, payload)
}

function assertRawPrivacy(body: string): void {
  for (const needle of ['R129_SYNTHETIC_PRIVATE_DOMAIN_CANARY', 'r131-synthetic-schedule-secret', '-----BEGIN PRIVATE KEY-----', '"apiKey":"test-only"']) {
    expect(body.includes(needle), 'raw corpus privacy before projection').toBe(false)
  }
}

function parseFrames(row: PublicRow): Array<{ block: string; event: PublicEvent }> {
  const frames: Array<{ block: string; event: PublicEvent }> = []
  let remaining = row.body
  while (remaining.trim()) {
    const next = takePublicRuntimeSseBlock(remaining)
    const block = next?.block ?? remaining
    remaining = next?.rest ?? ''
    const data = block.split(/\r?\n/).find((line) => line.startsWith('data:'))
    if (!data) continue
    frames.push({ block, event: JSON.parse(data.slice(5).trim()) as PublicEvent })
  }
  return frames
}

function mainEvent(block: string, threadId: string, corpus: Corpus): Record<string, unknown> {
  const decision = projectPublicRuntimeSseBlock(block, threadId, new PublicRuntimeEventFilter())
  const rejection = decision?.status === 'invalid' ? decision.reason : decision?.status ?? 'empty_frame'
  expect(decision?.status, `actual public Go SSE frame must enter Main projection: ${corpus.fault}/${rejection}`).toMatch(/^(emit|revoke)$/)
  if (!decision || (decision.status !== 'emit' && decision.status !== 'revoke')) throw new Error('Main rejected actual corpus')
  const event = decision.event
  if (event.kind === 'accepted_final_batch') {
    const verified = verifiedAcceptedFinalDeliveryBatch(event, corpus.publicLauncherAuthority)
    expect(verified, 'signature verified using actual handler public launcher identity').not.toBeNull()
    if (!verified) throw new Error('accepted final verification failed')
    return verified.batch
  }
  if (event.kind === 'general_terminal_batch') {
    const verification = generalTerminalDeliveryBatchVerificationV1(event)
    expect(verification.reason).toBeNull()
    if (!verification.verified) throw new Error('general terminal verification failed')
    return verification.verified.batch
  }
  return event
}

function makeStore(event: PublicEvent, stale = false): { get: () => ChatState; sink: ThreadEventSink } {
  let state = {
    activeThreadId: event.threadId, blocks: [], liveAssistant: '',
    lastSeq: event.firstSeq - (stale ? 2 : 1), busy: true, error: null,
    currentTurnId: event.turnId, currentTurnUserId: null,
    turnStartedAtByUserId: {}, turnDurationByUserId: {}, watchTurnCompletion: {}, unreadThreadIds: {},
    queuedMessages: [], threads: [], usageRefreshKey: 0,
    refreshThreads: async () => undefined, drainQueuedMessages: async () => undefined
  } as unknown as ChatState
  const get = (): ChatState => state
  const set: ChatStoreSet = (partial) => {
    state = { ...state, ...(typeof partial === 'function' ? partial(state) : partial) }
  }
  return { get, sink: buildThreadEventSink(set, get, { threadId: event.threadId, sinceSeq: state.lastSeq }) }
}

beforeAll(async () => {
  if (activation !== '1') throw new Error('invalid explicit public-consumer acceptance activation')
  if (!corpusRoot) throw new Error('actual Go corpus is required; no fixture fallback or skip')
  const cryptoEntry = 'node:crypto'
  const fsEntry = 'node:fs'
  const pathEntry = 'node:path'
  const mainEntry = '../../../main/runtime/analytix-adapter'
  const acceptedEntry = '../../../main/accepted-final-publication'
  const generalEntry = '../../../main/general-terminal-publication'
  const preloadEntry = '../../../preload/index'
  const crypto = await import(cryptoEntry)
  const fs = await import(fsEntry)
  digest = (body) => crypto.createHash('sha256').update(body).digest('hex')
  readFileSync = fs.readFileSync
  readdirSync = fs.readdirSync
  join = (await import(pathEntry)).join
  sanitizeRuntimeResponse = (await import(mainEntry)).sanitizeRuntimeResponse
  verifiedAcceptedFinalDeliveryBatch = (await import(acceptedEntry)).verifiedAcceptedFinalDeliveryBatch
  generalTerminalDeliveryBatchVerificationV1 = (await import(generalEntry)).generalTerminalDeliveryBatchVerificationV1
  expect(readdirSync(corpusRoot).sort()).toEqual(faults.map((fault) => `${fault}.json`).sort())
  for (const fault of faults) {
    const corpus = JSON.parse(readFileSync(join(corpusRoot, `${fault}.json`), 'utf8')) as Corpus
    expect(corpus.schemaVersion).toBe(1)
    expect(corpus.fault).toBe(fault)
    expect(corpus.privacyCheckedBeforeSampling).toBe(true)
    expect(corpus.drained).toBe(true)
    expect(corpus.responses.length).toBeGreaterThan(15)
    expect(corpus.operations).toHaveLength(fault === 'missing' ? 15 : 10)
    expect(corpus.input.commandId).toMatch(/^SHARED-/)
    expect(corpus.input.nodeVersion).toBe(runtimeProcess.version)
    for (const [path, expected] of Object.entries(corpus.input.files)) expect(digest(readFileSync(path)), 'producer source/tool identity').toBe(expected)
    for (const row of corpus.responses) {
      expect(digest(row.body)).toBe(row.sha256)
      assertRawPrivacy(row.body)
    }
    for (const operation of corpus.operations) {
      expect(operation.starts).toBe(1)
      expect(operation.ends).toBe(1)
      expect(operation.completionErrorClass).toBe('none')
      expect(operation.failureRecordErrorClass).toBe('none')
      expect(operation.terminalStatus).toBe('completed')
    }
    corpora.set(fault, corpus)
    expect(corpus.input).toEqual(corpora.get('missing')!.input)
  }
  await import(preloadEntry)
  api = electron.exposed.get('analytix') as AnalytixApi
  expect(api.runtime.runtimeRequest).toBeTypeOf('function')
  vi.stubGlobal('window', { analytix: api, setTimeout, clearTimeout, dispatchEvent: () => true, addEventListener: () => undefined, removeEventListener: () => undefined })
  vi.stubGlobal('requestAnimationFrame', (callback: () => void) => setTimeout(callback, 0))
  vi.stubGlobal('cancelAnimationFrame', clearTimeout)
  electron.invoke.mockImplementation(async (channel: string, args: unknown, ...rest: unknown[]) => {
    if (channel === 'runtime:request') {
      const request = args as { path: string; method: string }
      expect(responseRow).not.toBeNull()
      expect(request.path).toBe(responseRow!.path)
      expect(request.method ?? 'GET').toBe(responseRow!.method)
      return sanitizeRuntimeResponse({ ok: responseRow!.status < 400, status: responseRow!.status, body: responseRow!.body }, request.path, currentCorpus.publicLauncherAuthority, request.method)
    }
    if (channel === 'runtime:sse:start') {
      const request = args as { streamId: string; threadId: string }
      deliveredStream = request.streamId
      queueMicrotask(() => {
        for (const event of eventsToDeliver) emit('runtime:sse-event', { streamId: deliveredStream, events: [event] })
        queueMicrotask(() => emit('runtime:sse-end', { streamId: deliveredStream }))
      })
      return { streamId: deliveredStream }
    }
    if (channel === 'runtime:sse:ack') {
      const ack = args as { streamId: string; seq: number; batchId?: string; publicationCommitId?: string }
      expect(ack.streamId).toBe(deliveredStream)
      beforeAck?.(ack.seq, ack)
      ackCount++
      return true
    }
    if (channel === 'runtime:sse:stop') return true
    if (channel === 'notification:turn-complete') return { ok: true }
    // Notification/cache broadcasts are shell effects; no external sink runs.
    if (channel === 'query-cache:invalidate' || channel === 'workspace:release-thread-worktree') return true
    throw new Error(`unexpected shell IPC channel ${channel}, argument count ${rest.length}`)
  })
})

afterEach(() => {
  clearActiveStream()
  clearBusyWatchdog()
  resetPublicProjectionRevocationsForTests()
  rendererRuntimeClient.invalidateSettings()
  electron.listeners.clear()
  responseRow = null
  eventsToDeliver = []
  beforeAck = undefined
  ackCount = 0
})

describe.skipIf(activation === '').each(faults)(
  activation === '' ? 'NOT_CONFIGURED: Owner public-consumer acceptance inactive: %s' : 'actual Go public corpus: %s',
  (fault) => {
  it('hydrates complete ordinary work, listing, resume and restart through Main/preload/renderer', async () => {
    currentCorpus = corpora.get(fault)!
    const provider = new AnalytixRuntimeProvider()
    for (const phase of ['mcp', 'coding', 'writing', 'research', 'skills', 'jobs', 'post-denial', 'restart', 'resume']) {
      responseRow = currentCorpus.responses.find((row) => row.phase === phase && row.method === 'GET' && /^\/v1\/threads\/[^/?]+$/.test(row.path) && row.status === 200)!
      expect(responseRow, `actual detail for ${phase}`).toBeDefined()
      const raw = JSON.parse(responseRow.body) as { id: string; latestSeq: number }
      const detail = await provider.getThreadDetail(raw.id)
      expect(detail.latestSeq).toBe(raw.latestSeq)
      expect(detail.blocks.some((block) => block.kind === 'assistant' && block.text.includes(`R131_${phase.toUpperCase()}_COMPLETE`))).toBe(true)
      const threadId = raw.id
      responseRow = currentCorpus.responses.find((row) => row.phase === phase && row.path === '/v1/threads?limit=50' && row.status === 200)!
      expect(responseRow).toBeDefined()
      expect((await provider.listThreads({ limit: 50 })).some((thread) => thread.id === threadId)).toBe(true)
    }
    for (const row of currentCorpus.responses.filter((row) => row.phase === 'fresh-composition' && /^\/v1\/threads\/[^/?]+$/.test(row.path) && row.status === 200)) {
      responseRow = row
      const raw = JSON.parse(row.body) as { id: string; latestSeq: number }
      expect((await provider.getThreadDetail(raw.id)).latestSeq).toBe(raw.latestSeq)
    }
    for (const row of currentCorpus.responses.filter((row) => row.path.startsWith('/v1/runtime/task-jobs/') && row.status === 200)) {
      responseRow = row
      const result = await rendererRuntimeClient.runtimeRequest(row.path, row.method)
      expect(result.ok).toBe(true)
      if (row.path.endsWith('/output')) expect(JSON.parse(result.body)).toMatchObject({ availability: 'withheld', outputWithheld: true, canReadOutput: false, canContinueParent: false })
    }
  })

  it('verifies actual replay frames and commits terminal state before preload ACK', async () => {
    currentCorpus = corpora.get(fault)!
    const rows = currentCorpus.responses.filter((row) => row.path.includes('/events?') && row.status === 200)
    expect(rows.length).toBeGreaterThan(3)
    let accepted = 0
    let general = 0
    let revoked = 0
    let compacted = 0
    for (const row of rows) {
      for (const frame of parseFrames(row)) {
        const event = mainEvent(frame.block, frame.event.threadId, currentCorpus)
        if (event.kind === 'public_projection_revoked') { revoked++; continue }
        if (event.kind === 'context_compacted' || event.kind === 'compaction_completed') compacted++
        if (event.kind !== 'accepted_final_batch' && event.kind !== 'general_terminal_batch') continue
        if (event.kind === 'accepted_final_batch') accepted++
        else general++
        const harness = makeStore(frame.event)
        const terminalHandler = event.kind === 'accepted_final_batch' ? harness.sink.onAcceptedFinalBatch! : harness.sink.onGeneralTerminalBatch!
        let release!: () => void
        let entered!: () => void
        const started = new Promise<void>((resolve) => { entered = resolve })
        const gate = new Promise<void>((resolve) => { release = resolve })
        if (event.kind === 'accepted_final_batch') {
          const original = harness.sink.onAcceptedFinalBatch!
          harness.sink.onAcceptedFinalBatch = async (batch) => { entered(); await gate; return original(batch) }
        } else {
          const original = harness.sink.onGeneralTerminalBatch!
          harness.sink.onGeneralTerminalBatch = async (batch) => { entered(); await gate; return original(batch) }
        }
        expect(terminalHandler).toBeTypeOf('function')
        eventsToDeliver = [event]
        const before = ackCount
        beforeAck = (seq, binding) => {
          expect(harness.get().lastSeq).toBe(frame.event.lastSeq)
          expect(harness.get().busy).toBe(false)
          expect(harness.get().currentTurnId).toBeNull()
          expect(seq).toBe(frame.event.lastSeq)
          if (event.kind === 'accepted_final_batch') expect(binding).toMatchObject({ batchId: event.batchId, publicationCommitId: event.publicationCommitId, threadId: event.threadId, turnId: event.turnId })
          else expect(binding).not.toHaveProperty('batchId')
        }
        const subscription = new AnalytixRuntimeProvider().subscribeThreadEvents(frame.event.threadId, frame.event.firstSeq - 1, harness.sink, new AbortController().signal)
        await started
        expect(ackCount).toBe(before)
        expect(harness.get().busy).toBe(true)
        release()
        await subscription
        expect(ackCount).toBe(before + 1)
        expect(harness.get().error).toBeNull()
      }
    }
    expect(accepted).toBeGreaterThan(0)
    expect(general).toBeGreaterThan(0)
    expect(revoked).toBeGreaterThan(0)
    expect(compacted).toBeGreaterThan(0)
  }, 30_000)

  it('rejects malformed and mixed authority and stale terminal cursors from actual output', async () => {
    currentCorpus = corpora.get(fault)!
    const provider = new AnalytixRuntimeProvider()
    const detail = currentCorpus.responses.find((row) => row.phase === 'post-denial' && /^\/v1\/threads\/[^/?]+$/.test(row.path) && row.status === 200)!
    expect(detail).toBeDefined()
    const raw = JSON.parse(detail.body) as { id: string; turns: Array<{ items: Array<Record<string, unknown>> }> }
    responseRow = detail
    await expect(provider.getThreadDetail(raw.id), 'untampered public detail must hydrate before negative controls').resolves.toBeDefined()
    for (const change of ['malformed', 'mixed']) {
      const tampered = structuredClone(raw)
      const ordinary = tampered.turns.flatMap((turn) => turn.items).find((item) => item.kind === 'assistant_text' && item.ordinaryResult)
      expect(ordinary).toBeDefined()
      if (change === 'malformed') ordinary!.ordinaryResult = { schemaVersion: -1 }
      else ordinary!.acceptedFinal = { unauthorized: true }
      responseRow = { ...detail, body: JSON.stringify(tampered) }
      await expect(provider.getThreadDetail(raw.id)).rejects.toThrow()
    }
    const frame = currentCorpus.responses.filter((row) => row.path.includes('/events?')).flatMap(parseFrames).find(({ event }) => event.kind === 'general_terminal_batch')!
    expect(frame).toBeDefined()
    const tampered = structuredClone(frame.event)
    tampered.lastSeq++
    expect(generalTerminalDeliveryBatchVerificationV1(tampered).verified).toBeNull()
    expect(projectPublicRuntimeSseBlock(frame.block, 'different-thread', new PublicRuntimeEventFilter())?.status).toBe('invalid')
    const harness = makeStore(frame.event, true)
    const before = ackCount
    eventsToDeliver = [mainEvent(frame.block, frame.event.threadId, currentCorpus)]
    beforeAck = () => { throw new Error('stale terminal must not ACK') }
    await provider.subscribeThreadEvents(frame.event.threadId, frame.event.firstSeq - 2, harness.sink, new AbortController().signal)
    expect(ackCount).toBe(before)
    expect(harness.get().error).not.toBeNull()
  })
})
