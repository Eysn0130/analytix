import { spawn, type ChildProcessByStdio } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Readable } from 'node:stream'
import { describe, expect, it } from 'vitest'
import { findGoBinary, goTestEnv } from './go-test-toolchain.js'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(here, '..', '..', '..')
const runtimeGoDir = join(repoRoot, 'packages', 'runtime-go')
const durableContractUrl = new URL('../src/conformance/fixtures/go-durable-sidecar-contract.json', import.meta.url)
type GoTestChildProcess = ChildProcessByStdio<null, Readable, Readable>

type DurableContract = {
  runtimeToken: string
  threadId: string
  concurrentThreadId: string
  malformedLine: string
  eventDrafts: Array<Record<string, unknown>>
  concurrentEventCount: number
  expected: {
    eventKinds: string[]
    highestSeq: number
    replayAfterSeq: number
    replayAfterSeqCount: number
    caughtUpReplayFrameCount: number
    diagnosticCount: number
    providers: string[]
    totalCacheHitTokens: number
    totalCacheMissTokens: number
    approvalStatuses: string[]
    userInputStatuses: string[]
    mcpToolNames: string[]
    latestMcpFingerprint: string
    archiveThreadId: string
    forkThreadId: string
    resumeThreadId: string
  }
}

type SidecarHandle = {
  url: string
  runtimeToken: string
  process: GoTestChildProcess
  stderr: () => string
}

describe('Go temp durable sidecar conformance', () => {
  it('calls the Go sidecar and matches the TypeScript-owned durable contract', async () => {
    const contract = JSON.parse(await readFile(durableContractUrl, 'utf8')) as DurableContract
    const durableTempDir = await mkdtemp(join(tmpdir(), 'analytix-go-durable-'))
    const sidecar = await startGoSidecar(durableTempDir)
    try {
      const boundary = await sidecarJSON(sidecar, '/v1/conformance/durable/boundary')
      expect(boundary.tempDurableEventSessionStoreEnabled).toBe(true)
      expect(boundary.tempDirOnly).toBe(true)
      expect(boundary.defaultGoBackendEnabled).toBe(false)
      expect(boundary.rendererVisibleGoRoutesAllowed).toBe(false)
      expect(boundary.reasonixPublicProtocolAllowed).toBe(false)
      expect(boundary.realWorkspaceMutationAllowed).toBe(false)
      expect(boundary.credentialReadAllowed).toBe(false)

      const threads = await sidecarJSON(sidecar, '/v1/conformance/durable/threads')
      expect(threadIds(threads)).toContain('thr_g2_alpha')
      expect(threadIds(threads)).toContain('thr_g2_beta')
      expect(threadIds(threads)).toContain('thr_g2_read')

      const archived = await sidecarJSON(
        sidecar,
        `/v1/conformance/durable/threads/${contract.expected.archiveThreadId}`,
        { method: 'PATCH', body: JSON.stringify({ status: 'archived' }) }
      )
      expect(archived.status).toBe('archived')
      const archiveSearch = await sidecarJSON(
        sidecar,
        '/v1/conformance/durable/threads?include_archived=true&search=archive'
      )
      expect(threadIds(archiveSearch)).toContain(contract.expected.archiveThreadId)

      const fork = await sidecarJSON(sidecar, '/v1/conformance/durable/threads/thr_g2_read/fork', {
        method: 'POST',
        body: JSON.stringify({ relation: 'side', title: 'Durable side' }),
        expectedStatus: 201
      })
      expect(fork.id).toBe(contract.expected.forkThreadId)
      expect(fork.relation).toBe('side')
      const resume = await sidecarJSON(sidecar, '/v1/conformance/durable/sessions/thr_g2_read/resume', {
        method: 'POST',
        body: JSON.stringify({ workspace: '/tmp/durable-resume', model: 'deepseek-chat' }),
        expectedStatus: 201
      })
      expect(resume.thread_id).toBe(contract.expected.resumeThreadId)
      expect(resume.session_id).toBe('thr_g2_read')

      const recordedEvents = []
      for (const draft of contract.eventDrafts) {
        const response = await sidecarJSON(sidecar, '/v1/conformance/durable/event', {
          method: 'POST',
          body: JSON.stringify(draft)
        })
        expect(response.publishPersistOrder).toEqual(['persist', 'publish'])
        expect(response.newlineTerminated).toBe(true)
        recordedEvents.push(recordValue(response.event))
      }
      expect(recordedEvents.map((event) => event.kind)).toEqual(contract.expected.eventKinds)
      expect(recordedEvents.map((event) => event.seq)).toEqual(contract.eventDrafts.map((_, index) => index + 1))

      const concurrentResponses = await Promise.all(
        Array.from({ length: contract.concurrentEventCount }, (_, index) =>
          sidecarJSON(sidecar, '/v1/conformance/durable/event', {
            method: 'POST',
            body: JSON.stringify({
              kind: 'tool_catalog_changed',
              threadId: contract.concurrentThreadId,
              turnId: 'turn_concurrent',
              itemId: `item_${index}`
            })
          })
        )
      )
      const concurrentSeqs = concurrentResponses
        .map((response) => Number(recordValue(response.event).seq))
        .sort((a, b) => a - b)
      expect(concurrentSeqs).toEqual(Array.from({ length: contract.concurrentEventCount }, (_, index) => index + 1))

      await sidecarJSON(sidecar, '/v1/conformance/durable/events/malformed', {
        method: 'POST',
        body: JSON.stringify({ threadId: contract.threadId, line: contract.malformedLine }),
        expectedStatus: 400
      })
      const replay = await sidecarJSON(
        sidecar,
        `/v1/conformance/durable/events/replay?thread_id=${contract.threadId}&after_seq=0`
      )
      expect(arrayValue(replay.events)).toHaveLength(contract.eventDrafts.length)
      expect(arrayValue(replay.diagnostics)).toHaveLength(0)
      expect(replay.highestSeq).toBe(contract.expected.highestSeq)

      const queryFrames = await sidecarSSE(
        sidecar,
        `/v1/conformance/durable/threads/${contract.threadId}/events?since_seq=${contract.expected.replayAfterSeq}`
      )
      const headerFrames = await sidecarSSE(
        sidecar,
        `/v1/conformance/durable/threads/${contract.threadId}/events`,
        String(contract.expected.replayAfterSeq)
      )
      expect(queryFrames).toEqual(headerFrames)
      expect(queryFrames).toHaveLength(contract.expected.replayAfterSeqCount)
      expect(frameDigest(queryFrames)).toBe(frameDigest(headerFrames))
      const caughtUpFrames = await sidecarSSE(
        sidecar,
        `/v1/conformance/durable/threads/${contract.threadId}/events?since_seq=${contract.expected.highestSeq}`
      )
      expect(caughtUpFrames).toHaveLength(contract.expected.caughtUpReplayFrameCount)

      const state = await sidecarJSON(
        sidecar,
        `/v1/conformance/durable/recovered-state?thread_id=${contract.threadId}`
      )
      expect(state.eventKinds).toEqual(contract.expected.eventKinds)
      const usage = recordValue(state.usageCacheAccounting)
      expect(usage.providers).toEqual(contract.expected.providers)
      expect(usage.totalCacheHitTokens).toBe(contract.expected.totalCacheHitTokens)
      expect(usage.totalCacheMissTokens).toBe(contract.expected.totalCacheMissTokens)
      expect(usage.cacheTelemetryLost).toBe(false)
      expect(recordValue(state.approvals).statuses).toEqual(contract.expected.approvalStatuses)
      expect(recordValue(state.approvals).approvalExecutionUsed).toBe(false)
      expect(recordValue(state.userInputs).statuses).toEqual(contract.expected.userInputStatuses)
      expect(recordValue(state.userInputs).answersPersisted).toBe(false)
      expect(recordValue(state.mcp).toolNames).toEqual(contract.expected.mcpToolNames)
      expect(recordValue(state.mcp).latestFingerprint).toBe(contract.expected.latestMcpFingerprint)
      expect(recordValue(state.mcp).mcpConnectionUsed).toBe(false)

      const snapshot = await sidecarJSON(sidecar, '/v1/conformance/durable/snapshot')
      expect(snapshot.enabled).toBe(true)
      expect(snapshot.realWorkspaceWriteAllowed).toBe(false)
      expect(snapshot.credentialReadAllowed).toBe(false)
      expect(snapshot.defaultGoBackendEnabled).toBe(false)
    } finally {
      await stopSidecar(sidecar)
      await rm(durableTempDir, { recursive: true, force: true })
    }
  }, 120_000)
})

async function startGoSidecar(durableTempDir: string): Promise<SidecarHandle> {
  const go = await findGoBinary()
  const child = spawn(go, [
    'run',
    './cmd/contract-sidecar',
    '--addr',
    '127.0.0.1:0',
    '--fixtures-dir',
    '../runtime/src/conformance/fixtures',
    '--durable-temp-dir',
    durableTempDir
  ], {
    cwd: runtimeGoDir,
    env: goTestEnv(),
    stdio: ['ignore', 'pipe', 'pipe']
  })
  child.stdout.setEncoding('utf8')
  child.stderr.setEncoding('utf8')
  let stderr = ''
  child.stderr.on('data', (chunk) => {
    stderr += String(chunk)
  })
  const ready = await waitForReadyLine(child, () => stderr)
  return {
    url: ready.url,
    runtimeToken: ready.runtimeToken,
    process: child,
    stderr: () => stderr
  }
}

async function stopSidecar(sidecar: SidecarHandle): Promise<void> {
  if (sidecar.process.exitCode !== null) return
  sidecar.process.kill('SIGTERM')
  await new Promise<void>((resolve) => {
    const timeout = setTimeout(resolve, 2_000)
    sidecar.process.once('exit', () => {
      clearTimeout(timeout)
      resolve()
    })
  })
}

async function waitForReadyLine(
  child: GoTestChildProcess,
  stderr: () => string
): Promise<{ url: string; runtimeToken: string }> {
  return new Promise((resolve, reject) => {
    let stdout = ''
    const timer = setTimeout(() => {
      reject(new Error(`Go sidecar did not become ready. stderr:\n${stderr()}`))
    }, 60_000)
    child.once('exit', (code) => {
      clearTimeout(timer)
      reject(new Error(`Go sidecar exited before ready with code ${code}. stderr:\n${stderr()}`))
    })
    child.stdout.on('data', (chunk) => {
      stdout += String(chunk)
      const line = stdout.split('\n').find((item) => item.startsWith('ANALYTIX_SIDECAR_READY '))
      if (!line) return
      clearTimeout(timer)
      const parsed = JSON.parse(line.slice('ANALYTIX_SIDECAR_READY '.length)) as {
        url: string
        runtimeToken: string
      }
      resolve(parsed)
    })
  })
}

async function sidecarJSON(
  sidecar: SidecarHandle,
  path: string,
  options: {
    method?: string
    body?: string
    expectedStatus?: number
  } = {}
): Promise<Record<string, unknown>> {
  const response = await fetch(`${sidecar.url}${path}`, {
    method: options.method ?? 'GET',
    headers: {
      authorization: `Bearer ${sidecar.runtimeToken}`,
      ...(options.body ? { 'content-type': 'application/json' } : {})
    },
    body: options.body
  })
  expect(response.status).toBe(options.expectedStatus ?? 200)
  return recordValue(await response.json())
}

async function sidecarSSE(sidecar: SidecarHandle, path: string, lastEventId?: string): Promise<string[]> {
  const response = await fetch(`${sidecar.url}${path}`, {
    headers: {
      authorization: `Bearer ${sidecar.runtimeToken}`,
      ...(lastEventId ? { 'last-event-id': lastEventId } : {})
    }
  })
  expect(response.status).toBe(200)
  expect(response.headers.get('content-type') ?? '').toContain('text/event-stream')
  const body = await response.text()
  return body.trim() ? body.trim().split('\n\n') : []
}

function threadIds(response: Record<string, unknown>): string[] {
  return arrayValue(response.threads)
    .map((item) => recordValue(item).id)
    .filter((id): id is string => typeof id === 'string')
    .sort()
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function frameDigest(frames: string[]): string {
  return createHash('sha256').update(frames.join('\n\n')).digest('hex')
}
