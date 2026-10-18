// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { CompactResponse } from '../../../../packages/runtime/src/contracts/turns'
import type { PipelineStage, RuntimeEventKind } from '../../../../packages/runtime/src/contracts/events'
import type { SseErrorPayload } from '@shared/analytix-api'
import { isStrictPublicRuntimeSseErrorPayload, type PublicRuntimeSseDecodeDiagnostic } from '@shared/public-runtime-sse'
import {
  defaultClawSettings, defaultKeyboardShortcuts, defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings, defaultScheduleSettings, defaultWriteSettings, type AppSettingsV1
} from '@shared/app-settings'
import { AnalytixRuntimeProvider } from '../agent/analytix-runtime'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { isOrdinaryDisplayHistoryBlock } from '../agent/analytix-mapper'
import { isInternalTemporaryWorkspace } from '../lib/workspace-path'
import { resetProviderCacheForTests } from '../agent/registry'
import { MessageTimeline } from '../components/chat/MessageTimeline'
import { clearBusyWatchdog, stopTurnCompletionPoll } from './chat-store-schedulers'
import { useChatStore } from './chat-store'

vi.mock('electron', () => ({ app: { getPath: () => { throw new Error('Unexpected live Electron profile access.') } } }))

type RejectedSseFrame = {
  kind: RuntimeEventKind | 'unknown'
  seq?: number
  schemaVersion?: number
  diagnostic?: PublicRuntimeSseDecodeDiagnostic
  pipeline?: {
    stage: PipelineStage | 'provider_admission_rejected' | 'unknown'
    presentFields: boolean[]
    detailsKind: 'record' | 'absent' | 'other'
    logicalLabelMatches?: boolean
    logicalIdCanonical?: boolean
    logicalDetailsValid?: boolean
    unexpectedLogicalEnvelopeFields?: number
  }
}
const rejectedSseFrames = vi.hoisted(() => [] as RejectedSseFrame[])
vi.mock('@shared/public-runtime-sse', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@shared/public-runtime-sse')>()
  const { RuntimeEventKind, PipelineStage, LogicalStepStartedDetailsV1Schema, LogicalStepClosedDetailsV1Schema } =
    await import('../../../../packages/runtime/src/contracts/events')
  return { ...actual, projectPublicRuntimeSseBlock: (...args: Parameters<typeof actual.projectPublicRuntimeSseBlock>) => {
    const [block, expectedThreadId, filter, onDiagnostic] = args
    let metadata: Omit<RejectedSseFrame, 'diagnostic'> = { kind: 'unknown' }
    let captured = false
    let diagnostic: PublicRuntimeSseDecodeDiagnostic | undefined
    const originalParse = JSON.parse
    // Plain forwarding functions keep no mock history of frame text or parsed records.
    // Observe the decoder's existing synchronous parse; do not parse the frame again.
    JSON.parse = function (this: unknown, ...parseArgs: Parameters<typeof JSON.parse>) {
      const value: unknown = Reflect.apply(originalParse, this, parseArgs)
      try {
        if (!captured) {
          captured = true
          if (value && typeof value === 'object' && !Array.isArray(value)) {
            const event = value as Record<string, unknown>
            const kind = RuntimeEventKind.safeParse(event.kind)
            metadata = { kind: kind.success ? kind.data : 'unknown',
              ...(Number.isSafeInteger(event.seq) ? { seq: event.seq as number } : {}),
              ...(Number.isSafeInteger(event.schemaVersion) ? { schemaVersion: event.schemaVersion as number } : {}) }
            if (event.kind === 'pipeline_stage') {
              const stage = PipelineStage.safeParse(event.stage)
              const details = event.details && typeof event.details === 'object' && !Array.isArray(event.details)
                ? event.details as Record<string, unknown> : undefined
              // Retain only a closed stage classification and structural facts,
              // never IDs, raw labels, unknown field names, prose or frame text.
              metadata.pipeline = {
                stage: stage.success ? stage.data : event.stage === 'provider_admission_rejected'
                  ? 'provider_admission_rejected' : 'unknown',
                // Fixed order: turnId, itemId, trace, message, child, attempt, maxAttempt.
                presentFields: ['turnId', 'itemId', 'trace', 'message', 'child', 'attempt', 'maxAttempt']
                  .map(key => Object.prototype.hasOwnProperty.call(event, key)),
                detailsKind: details ? 'record' : event.details === undefined ? 'absent' : 'other'
              }
              const logicalLabels = { logical_step_started: 'Logical step started',
                logical_step_assembled: 'Logical step assembled', logical_step_closed: 'Logical step closed' }
              if (stage.success && (stage.data === 'logical_step_started' || stage.data === 'logical_step_assembled' ||
                  stage.data === 'logical_step_closed')) {
                metadata.pipeline.logicalLabelMatches = event.label === logicalLabels[stage.data]
                metadata.pipeline.logicalIdCanonical = LogicalStepStartedDetailsV1Schema.safeParse({
                  schemaVersion: 1, logicalStepId: details?.logicalStepId }).success
                metadata.pipeline.logicalDetailsValid = (stage.data === 'logical_step_closed'
                  ? LogicalStepClosedDetailsV1Schema : LogicalStepStartedDetailsV1Schema).safeParse(event.details).success
                metadata.pipeline.unexpectedLogicalEnvelopeFields = Object.keys(event).filter(key => ![
                  'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'stage', 'label', 'details'
                ].includes(key)).length
              }
            }
          }
        }
      } catch { /* Diagnostic observation must not change parsing. */ }
      return value
    }
    let result: ReturnType<typeof actual.projectPublicRuntimeSseBlock>
    try {
      result = actual.projectPublicRuntimeSseBlock(block, expectedThreadId, filter, onDiagnostic && (value => {
        try {
          diagnostic = { validation: value.validation, stage: value.stage, reason: value.reason, fieldPaths: [...value.fieldPaths] }
        } catch { /* Always forward the original observer. */ }
        onDiagnostic(value)
      }))
    } finally {
      JSON.parse = originalParse
    }
    if (result?.status === 'invalid' || result?.status === 'withheld') {
      try {
        const rejection = { ...metadata, ...(diagnostic ? { diagnostic } : {}) }
        rejectedSseFrames.push(rejection)
        console.info('COMPACTION_HISTORY_REJECTED_FRAME', JSON.stringify(rejection))
      } catch { /* Diagnostic observation must not change the decoder result. */ }
    }
    return result
  } }
})

type Child = {
  pid?: number
  stdout: { on: (event: string, handler: (chunk: unknown) => void) => void }
  stderr: { on: (event: string, handler: (chunk: unknown) => void) => void }
  once: (event: string, handler: (value: unknown, signal?: unknown) => void) => void
  kill: (signal: string) => boolean
}
type Handler = (event: { sender: unknown }, args: unknown) => Promise<unknown>
type RuntimeApi = Window['analytix']['runtime']
const record = (value: unknown): Record<string, unknown> => value && typeof value === 'object'
  ? value as Record<string, unknown> : {}
const list = (value: unknown): Record<string, unknown>[] => Array.isArray(value) ? value.map(record) : []

describe('closed CompactResponse production compatibility', () => {
  const legacy = { threadId: 'thread-contract', replacedTokens: 12, summary: 'Prior conversation compacted.',
    pinnedConstraints: ['user: preserve recent turns'], sourceDigest: 'a'.repeat(64),
    digestMarker: `sha256:${'a'.repeat(12)}`, sourceItemIds: ['item-source'] }
  const completed = { ...legacy, ok: true, turnId: 'turn-compact', itemId: 'item-compact', eventSeq: 2,
    schemaVersion: 2, reasoningExcluded: true, reasoningExclusionProof: `sha256:${'b'.repeat(64)}` }
  const noop = { ok: true, threadId: 'thread-contract', replacedTokens: 0, pinnedConstraints: [] }
  it.each([['Go completed', completed], ['Go noop', noop], ['legacy', legacy]])('preserves exact %s envelope', (_, value) => {
    expect(CompactResponse.parse(value)).toEqual(value)
  })
  it.each([
    ['completed extra field', { ...completed, extra: true }],
    ['noop extra field', { ...noop, summary: 'Invented summary' }],
    ['legacy extra field', { ...legacy, extra: true }],
    ['invalid proof', { ...completed, reasoningExclusionProof: 'sha256:invalid' }],
    ['wrong schema version', { ...completed, schemaVersion: 3 }],
    ['reasoning exclusion absent', { ...completed, reasoningExcluded: undefined }],
    ['reasoning exclusion false', { ...completed, reasoningExcluded: false }],
    ['negative sequence', { ...completed, eventSeq: -1 }],
    ['zero sequence', { ...completed, eventSeq: 0 }],
    ['fractional sequence', { ...completed, eventSeq: 1.5 }],
    ['unsafe sequence', { ...completed, eventSeq: Number.MAX_SAFE_INTEGER + 1 }],
    ['partial completed envelope', { ...completed, itemId: undefined }],
    ['completed zero replacement', { ...completed, replacedTokens: 0 }],
    ['noop nonzero replacement', { ...noop, replacedTokens: 1 }]
  ])('rejects %s', (_, value) => {
    expect(CompactResponse.safeParse(value).success).toBe(false)
  })
})

// This is a production server + Host IPC + actual store + actual Timeline test.
// The only Provider is an owned loopback fixture; no live credentials or profile
// are read, and no event IDs or recovered transcript are handwritten.
it('restores publicly authorized ordinary history after production compaction and restart without enlarging model primary', async () => {
  rejectedSseFrames.length = 0
  const fs = await vi.importActual<{
    appendFile: (path: string, value: string) => Promise<void>
    mkdtemp: (prefix: string) => Promise<string>
    readFile: (path: string) => Promise<Uint8Array>
    mkdir: (path: string, options: { recursive: boolean; mode: number }) => Promise<void>
    writeFile: (path: string, value: Uint8Array | string, options: { mode: number; flag: string }) => Promise<void>
    rm: (path: string, options: { recursive: boolean; force: boolean }) => Promise<void>
  }>('node:fs/promises')
  const proc = await vi.importActual<{ env: Record<string, string | undefined>; cwd: () => string; platform: string;
    kill: (pid: number, signal: string) => boolean }>('node:process')
  const { tmpdir } = await vi.importActual<{ tmpdir: () => string }>('node:os')
  const { createHash } = await vi.importActual<{
    createHash: (algorithm: string) => { update: (value: Uint8Array) => { digest: (encoding: string) => string } }
  }>('node:crypto')
  const { spawn } = await vi.importActual<{
    spawn: (binary: string, args: string[], options: { cwd: string; env: Record<string, string | undefined>; stdio: string[]; detached?: boolean }) => Child
  }>('node:child_process')
  const { findGoBinary, goTestEnv } = await vi.importActual<{
    findGoBinary: () => Promise<string>
    goTestEnv: (extra?: Record<string, string>) => Record<string, string | undefined>
  }>('../../../../packages/runtime/tests/go-test-toolchain')
  const { createServer } = await vi.importActual<{
    createServer: (handler: (request: { on: (event: string, handler: (chunk?: unknown) => void) => void },
      response: { writeHead: (code: number, headers: Record<string, string>) => void; end: (value: string) => void }) => void) => {
      listen: (port: number, host: string, ready: () => void) => void
      address: () => { port: number } | null
      close: (done: () => void) => void
    }
  }>('node:http')
  const { registerRuntimeSseIpc } = await vi.importActual<{
    registerRuntimeSseIpc: (options: { ipcMain: { handle: (channel: string, handler: Handler) => void };
      store: { load: () => Promise<AppSettingsV1> }; ensureRuntime: () => Promise<void>;
      logError: (category: string, message: string, detail?: unknown) => void }) => () => Promise<void>
  }>('../../../main/runtime-sse-ipc')
  const { runtimeRequestViaHost } = await vi.importActual<{
    runtimeRequestViaHost: (settings: AppSettingsV1, path: string, init: { method?: string; body?: string },
      ensure: () => Promise<void>) => Promise<{ ok: boolean; status: number; body: string }>
  }>('../../../main/runtime/analytix-adapter')
  const sourceInputSha256 = proc.env.ANALYTIX_HISTORY_GO_SOURCE_SHA256
  const goInputsPath = proc.env.ANALYTIX_HISTORY_GO_INPUT_MANIFEST
  const validationReceiptPath = proc.env.ANALYTIX_HISTORY_VALIDATION_RECEIPT
  const reusePath = proc.env.ANALYTIX_HISTORY_REUSE_BUILD_RECEIPT
  const qualificationInputs = [sourceInputSha256, goInputsPath, validationReceiptPath, reusePath]
  const qualifiedBuild = qualificationInputs.some(value => value !== undefined)
  if (qualifiedBuild && (qualificationInputs.some(value => value !== undefined && !value.trim()) ||
    typeof sourceInputSha256 !== 'string' || !/^[a-f0-9]{64}$/.test(sourceInputSha256) ||
    !goInputsPath || !(validationReceiptPath || reusePath))) {
    throw new Error('Qualified fixture retention or reuse requires a verified source digest, Go input manifest, and retention or reuse receipt path.')
  }
  let fixtureRoot = '', stateRoot = '', dataDir = '', workspaceRoot = '', executable = '', binaryRoot = ''
  const receipt: Record<string, unknown> = { sourceInputSha256,
    buildTags: ['analytix_prod'], runtimePids: [], runtimeUrls: [] }
  let acceptancePhase = 'setup', diagnosticCount = 0, workerCrashed = false, acceptanceCompleted = false
  let waitForSseWorkers: (() => Promise<void>) | undefined
  const goDir = `${proc.cwd()}/packages/runtime-go`
  const token = 'isolated-compaction-history-runtime'
  const providerRequests: string[] = []
  let holdNextResponse = false
  let releaseHeldResponse: (() => void) | undefined
  const providerServer = createServer((req, res) => {
    let body = ''
    req.on('data', chunk => { body += String(chunk) })
    req.on('end', () => {
      providerRequests.push(body)
      const text = `Ordinary fixture answer ${providerRequests.length}.`
      res.writeHead(200, { 'content-type': 'text/event-stream' })
      const finish = () => res.end([
        `data: ${JSON.stringify({ choices: [{ delta: { content: text }, finish_reason: 'stop' }] })}`,
        `data: ${JSON.stringify({ choices: [], usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 } })}`,
        'data: [DONE]', ''
      ].join('\n\n'))
      if (holdNextResponse) releaseHeldResponse = finish
      else finish()
    })
  })
  let stopRuntime: (() => Promise<void>) | undefined
  let buildClosed = true, acceptanceStartedAt = 0
  const originalBridge = Object.getOwnPropertyDescriptor(window, 'analytix')
  const initialState = useChatStore.getInitialState()
  const container = document.createElement('div')
  document.body.append(container)
  const uiRoot = createRoot(container)
  const handlers = new Map<string, Handler>()
  const dataListeners = new Set<Parameters<RuntimeApi['onSseEvent']>[0]>()
  const endListeners = new Set<Parameters<RuntimeApi['onSseEnd']>[0]>()
  const errorListeners = new Set<Parameters<RuntimeApi['onSseError']>[0]>()
  const sseErrorDiagnostics: Array<Pick<SseErrorPayload, 'code' | 'status' | 'reasonCode'>> = []
  const decodeDiagnostics: PublicRuntimeSseDecodeDiagnostic[] = []
  const streams = new Set<string>()
  const pendingStarts = new Set<Promise<{ streamId: string }>>()
  const sender = { id: 9001, isDestroyed: () => false, send: (channel: string, payload: unknown) => {
    if (channel === 'runtime:sse-event') dataListeners.forEach(fn => fn(payload as Parameters<typeof fn>[0]))
    if (channel === 'runtime:sse-end') endListeners.forEach(fn => fn(payload as Parameters<typeof fn>[0]))
    if (channel === 'runtime:sse-error') {
      if (isStrictPublicRuntimeSseErrorPayload(payload)) {
        const { code, status, reasonCode } = payload
        sseErrorDiagnostics.push({ code, status, reasonCode })
      }
      errorListeners.forEach(fn => fn(payload as Parameters<typeof fn>[0]))
    }
  } }
  const stopStreams = async () => {
    for (const streamId of streams) await handlers.get('runtime:sse:stop')?.({ sender }, streamId)
    streams.clear()
  }
  const waitFor = async (condition: () => Promise<boolean>) => {
    for (let count = 0; count < 200; count++) {
      if (await condition()) return
      await new Promise<void>(resolve => setTimeout(resolve, 25))
    }
    throw new Error('Production fixture did not reach the required state.')
  }
  try {
    fixtureRoot = await fs.mkdtemp(`${tmpdir()}/analytix-compaction-history-`)
    // Runtime state still passes the internal APFS authority checks.
    stateRoot = await fs.mkdtemp(proc.platform === 'darwin'
      ? '/private/tmp/analytix-compaction-history-state-' : `${tmpdir()}/analytix-compaction-history-state-`)
    dataDir = `${stateRoot}/data`
    // Ordinary workspace identity must remain outside the OS-internal temp roots.
    const workspaceFixturesRoot = `${proc.cwd()}/.cache`
    await fs.mkdir(workspaceFixturesRoot, { recursive: true, mode: 0o700 })
    workspaceRoot = await fs.mkdtemp(`${workspaceFixturesRoot}/analytix-compaction-history-workspace-`)
    if (!reusePath) {
      binaryRoot = await fs.mkdtemp(`${tmpdir()}/analytix-compaction-history-build-`)
      executable = `${binaryRoot}/runtime-server${proc.platform === 'win32' ? '.exe' : ''}`
    }
    receipt.stateRoot = stateRoot; receipt.workspaceRoot = workspaceRoot
    expect(isInternalTemporaryWorkspace(workspaceRoot)).toBe(false)
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
    vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
    await fs.mkdir(workspaceRoot, { recursive: true, mode: 0o700 })
    await new Promise<void>(resolve => providerServer.listen(0, '127.0.0.1', resolve))
    const providerUrl = `http://127.0.0.1:${providerServer.address()!.port}/v1`
    receipt.providerUrl = providerUrl
    const masterDir = `${dataDir}/private/provider-secrets/master-key`
    await fs.mkdir(masterDir, { recursive: true, mode: 0o700 })
    // Existing synthetic Secret Store fixture format, in this task-owned root.
    await fs.writeFile(`${masterDir}/master.key`, new Uint8Array(32).fill(0x6a), { mode: 0o600, flag: 'wx' })
    if (proc.platform === 'darwin') await fs.writeFile(`${masterDir}/authority.v1`,
      'analytix-master-key-authority:v1:fallback\n', { mode: 0o600, flag: 'wx' })
    const go = await findGoBinary()
    const buildArgs = ['build', '-v', '-x', '-tags', 'analytix_prod', '-o', executable, './cmd/runtime-server']
    receipt.goBinary = go; receipt.buildCwd = goDir
    receipt.goBinarySha256 = createHash('sha256').update(await fs.readFile(go)).digest('hex')
    let goInputs: Record<string, unknown> | undefined
    if (qualifiedBuild) {
      const goInputsBytes = await fs.readFile(goInputsPath!)
      goInputs = record(JSON.parse(new TextDecoder().decode(goInputsBytes)))
      if (goInputs.sourceInputSha256 !== receipt.sourceInputSha256 || goInputs.platform !== proc.platform) {
        throw new Error('Fixture Go input digest or platform mismatch.')
      }
      receipt.goInputManifestSha256 = createHash('sha256').update(goInputsBytes).digest('hex')
    }
    if (reusePath) {
      const previous = record(JSON.parse(new TextDecoder().decode(await fs.readFile(reusePath))))
      if (previous.sourceInputSha256 !== receipt.sourceInputSha256 ||
        previous.goBinarySha256 !== receipt.goBinarySha256 ||
        previous.goInputManifestSha256 !== receipt.goInputManifestSha256 ||
        JSON.stringify(previous.buildTags) !== JSON.stringify(['analytix_prod']) ||
        previous.buildExitCode !== 0 || previous.buildCloseCode !== 0 || typeof previous.binaryPath !== 'string') {
        throw new Error('Retained fixture binary does not match the qualified production inputs.')
      }
      executable = previous.binaryPath
      const digest = createHash('sha256').update(await fs.readFile(executable)).digest('hex')
      if (digest !== previous.binarySha256) throw new Error('Retained fixture binary digest mismatch.')
      receipt.binarySha256 = digest; receipt.binaryPath = executable
      receipt.binaryInputsReceipt = reusePath; receipt.binaryInputsReceiptSha256 =
        createHash('sha256').update(await fs.readFile(reusePath)).digest('hex')
      receipt.reusedQualifiedBuild = true
    } else {
      receipt.buildArgs = buildArgs
      const buildStartedAt = Date.now()
      const buildLogPrefix = validationReceiptPath ?? `${fixtureRoot}/go-build`
      const stdoutLog = `${buildLogPrefix}.build.stdout.log`, stderrLog = `${buildLogPrefix}.build.stderr.log`
      await fs.writeFile(stdoutLog, '', { mode: 0o600, flag: 'wx' })
      await fs.writeFile(stderrLog, '', { mode: 0o600, flag: 'wx' })
      receipt.buildStartedAt = new Date(buildStartedAt).toISOString()
      receipt.buildLimitsMs = { total: 600_000, withoutCompilerProgress: 180_000, cancellationClose: 60_000 }
      receipt.buildStdoutLog = stdoutLog; receipt.buildStderrLog = stderrLog
      const build = spawn(go, buildArgs,
        { cwd: goDir, env: goTestEnv(), stdio: ['ignore', 'pipe', 'pipe'], detached: proc.platform !== 'win32' })
      buildClosed = false
      receipt.buildPid = build.pid; receipt.buildProcessGroup = proc.platform === 'win32' ? null : build.pid
      let logWrites = Promise.resolve(), logError: unknown, progressBuffer = '', lastProgressAt = buildStartedAt
      const compilerCommands = new Set<string>()
      let compilerActions = 0, cancelReason = '', cancellationFailed: (() => void) | undefined
      const cancellationFailure = new Promise<void>((_, reject) => {
        cancellationFailed = () => reject(new Error('Fixture Go build did not close after normal cancellation.'))
      })
      const capture = (path: string, chunk: unknown) => {
        const output = String(chunk)
        logWrites = logWrites.then(() => fs.appendFile(path, output)).catch(error => { logError = error })
        progressBuffer += output
        let end = progressBuffer.indexOf('\n')
        while (end >= 0) {
          const line = progressBuffer.slice(0, end); progressBuffer = progressBuffer.slice(end + 1)
          // -x exposes actual compiler/assembler/linker invocations, including
          // a cache-warm final link. Arbitrary output does not reset this clock.
          if (/\/pkg\/tool\/[^/\s]+\/(compile|asm|cgo|link)\s/.test(line) && !compilerCommands.has(line)) {
            compilerCommands.add(line); compilerActions++; lastProgressAt = Date.now()
            receipt.buildCompilerActions = compilerActions
            receipt.buildLastCompilerProgressAt = new Date(lastProgressAt).toISOString()
            console.info('COMPACTION_HISTORY_BUILD_PROGRESS', JSON.stringify({ pid: build.pid,
              compilerActions, elapsedMs: lastProgressAt - buildStartedAt }))
          }
          end = progressBuffer.indexOf('\n')
        }
      }
      build.stdout.on('data', chunk => capture(stdoutLog, chunk))
      build.stderr.on('data', chunk => capture(stderrLog, chunk))
      const closed = new Promise<void>(resolve => {
        build.once('error', error => { receipt.buildSpawnError = String(error) })
        build.once('exit', (code, signal) => {
          receipt.buildExitCode = code; receipt.buildExitSignal = signal
          receipt.buildExitedAt = new Date().toISOString()
        })
        build.once('close', (code, signal) => {
          buildClosed = true; receipt.buildCloseCode = code; receipt.buildCloseSignal = signal
          receipt.buildClosedAt = new Date().toISOString(); resolve()
        })
      })
      let cancellationTimer: ReturnType<typeof setTimeout> | undefined
      const supervision = setInterval(() => {
        const now = Date.now()
        if (cancelReason || buildClosed) return
        if (now - buildStartedAt >= 600_000) cancelReason = 'total_build_deadline'
        else if (now - lastProgressAt >= 180_000) cancelReason = 'no_compiler_progress'
        if (!cancelReason) return
        receipt.buildCancellation = { reason: cancelReason, at: new Date(now).toISOString(), signal: 'SIGINT' }
        try {
          // detached establishes this exact child as its own process group.
          receipt.buildCancellationSignalSent = proc.platform !== 'win32' && build.pid
            ? proc.kill(-build.pid, 'SIGINT') : build.kill('SIGINT')
        } catch (error) { receipt.buildCancellationSignalError = String(error) }
        cancellationTimer = setTimeout(() => cancellationFailed?.(), 60_000)
      }, 1000)
      try { await Promise.race([closed, cancellationFailure]) } finally {
        clearInterval(supervision); if (cancellationTimer) clearTimeout(cancellationTimer)
        await logWrites
        receipt.buildDurationMs = Date.now() - buildStartedAt; receipt.buildProcessClosed = buildClosed
        if (buildClosed) {
          receipt.buildStdoutSha256 = createHash('sha256').update(await fs.readFile(stdoutLog)).digest('hex')
          receipt.buildStderrSha256 = createHash('sha256').update(await fs.readFile(stderrLog)).digest('hex')
        } else receipt.buildOutputClosure = 'unclosed; no final log hash claimed'
        console.info('COMPACTION_HISTORY_BUILD_RECEIPT', JSON.stringify(receipt))
      }
      if (logError) throw new Error('Fixture Go build output could not be retained.')
      if (cancelReason) throw new Error(`Fixture Go build cancelled: ${cancelReason}.`)
      if (receipt.buildSpawnError || receipt.buildExitCode !== 0 || receipt.buildCloseCode !== 0) {
        throw new Error(`Fixture Go build failed; see ${stderrLog}.`)
      }
      receipt.binarySha256 = createHash('sha256').update(await fs.readFile(executable)).digest('hex')
      receipt.binaryPath = executable
      if (qualifiedBuild) {
        const inputsPath = `${binaryRoot}/inputs.json`
        await fs.writeFile(inputsPath, JSON.stringify({ ...receipt, goInputs }, null, 2) + '\n', { mode: 0o600, flag: 'wx' })
        receipt.binaryInputsReceipt = inputsPath
        receipt.binaryInputsReceiptSha256 = createHash('sha256').update(await fs.readFile(inputsPath)).digest('hex')
      }
    }
    acceptanceStartedAt = Date.now()
    receipt.acceptanceStartedAt = new Date(acceptanceStartedAt).toISOString()
    console.info('COMPACTION_HISTORY_BINARY_RECEIPT', JSON.stringify(receipt))
    const startRuntime = async (): Promise<string> => {
      const child = spawn(executable, ['--addr', '127.0.0.1:0', '--data-dir', dataDir, '--durable-root', dataDir,
        '--provider-id', 'deepseek', '--model', 'deepseek-chat', '--endpoint-format', 'chat_completions'],
      { cwd: goDir, env: goTestEnv({ ANALYTIX_RUNTIME_TOKEN: token }), stdio: ['ignore', 'pipe', 'pipe'] })
      ;(receipt.runtimePids as Array<number | undefined>).push(child.pid)
      let closed = false
      const close = new Promise<void>(resolve => {
        child.once('close', () => { closed = true; resolve() })
      })
      stopRuntime = async () => {
        if (closed) return
        child.kill('SIGTERM')
        const killTimer = setTimeout(() => child.kill('SIGKILL'), 2000)
        await close; clearTimeout(killTimer)
      }
      return new Promise<string>((resolve, reject) => {
        let output = '', ready = false
        const timer = setTimeout(() => reject(new Error('Fixture runtime readiness timed out.')), 30_000)
        child.stderr.on('data', () => undefined)
        child.once('error', () => { clearTimeout(timer); reject(new Error('Fixture runtime could not start.')) })
        child.once('exit', () => { clearTimeout(timer); if (!ready) reject(new Error('Fixture runtime exited before readiness.')) })
        child.stdout.on('data', chunk => {
          output += String(chunk)
          const line = output.split('\n').find(value => value.startsWith('ANALYTIX_RUNTIME_SERVER_READY '))
          if (!line || ready) return
          ready = true; clearTimeout(timer)
          const url = String(record(JSON.parse(line.slice('ANALYTIX_RUNTIME_SERVER_READY '.length))).url)
          ;(receipt.runtimeUrls as string[]).push(url)
          resolve(url)
        })
      })
    }
    let runtimeUrl = await startRuntime()
    const json = async (path: string, body?: unknown): Promise<Record<string, unknown>> => {
      const response = await fetch(`${runtimeUrl}${path}`, { method: body === undefined ? 'GET' : 'POST',
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }) })
      if (!response.ok) throw new Error(`Production fixture HTTP failed (${response.status}).`)
      return record(await response.json())
    }
    const registry = await json('/v1/provider-registry')
    await json('/v1/provider-registry', { schemaVersion: 1,
      expected: { registryRevision: registry.registryRevision, registryIncarnation: registry.registryIncarnation,
        providerRevision: '0', providerGeneration: '0', providerIncarnation: '', providerCredentialPurpose: '' },
      provider: { id: 'deepseek', kind: 'openai-compatible', endpoint: providerUrl, proxy: '', models: ['deepseek-chat'],
        mediaModels: [], selectedModel: 'deepseek-chat', selectedMediaModel: '', selectedRoutes: ['primary'] },
      credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: btoa('synthetic-loopback-only') } })
    const thread = await json('/v1/threads', { workspace: workspaceRoot, title: 'Ordinary history fixture', model: 'deepseek-chat' })
    const threadId = String(thread.id), path = `/v1/threads/${encodeURIComponent(threadId)}`
    const prompts = Array.from({ length: 26 }, (_, index) => `Implement ordinary fixture request ${index + 1}.`)
    const turnsStartedAt = Date.now()
    acceptancePhase = 'fixture_turns'
    for (const prompt of prompts) {
      const started = await json(`${path}/turns`, { prompt, model: 'deepseek-chat', providerId: 'deepseek' })
      await waitFor(async () => list((await json(path)).turns).some(turn => turn.id === started.turnId && turn.status === 'completed'))
    }
    receipt.completedFixtureTurns = prompts.length; receipt.fixtureTurnsDurationMs = Date.now() - turnsStartedAt
    const settings: AppSettingsV1 = { version: 1, locale: 'en', theme: 'system', uiFontScale: 'small',
      provider: defaultModelProviderSettings(), runtime: { ...defaultAnalytixRuntimeSettings(Number(new URL(runtimeUrl).port)), baseUrl: runtimeUrl,
        runtimeToken: token, autoStart: false, dataDir }, workspaceRoot,
      log: { enabled: false, retentionDays: 7 }, notifications: { turnComplete: false },
      appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
      keyboardShortcuts: defaultKeyboardShortcuts(), write: defaultWriteSettings(), claw: defaultClawSettings(),
      schedule: defaultScheduleSettings(), guiUpdate: { channel: 'stable' }, codePromptPrefix: '', disabledSkillIds: [] }
    waitForSseWorkers = registerRuntimeSseIpc({ ipcMain: { handle: (channel, handler) => { handlers.set(channel, handler) } },
      store: { load: async () => settings }, ensureRuntime: async () => undefined,
      logError: (category, message, detail) => {
        if (category === 'sse' && message === 'SSE worker crashed') workerCrashed = true
        if (category === 'sse' && message === 'SSE first failure' && diagnosticCount < 32) {
          // Main supplies closed categories only; never forward arbitrary log detail.
          const { role, phase, reason, errorName, causeCode, elapsedMs, statusCode } = record(detail)
          const diagnostic = { acceptancePhase, role, phase, reason, errorName, causeCode, elapsedMs, statusCode }
          receipt.firstSseFailure ??= diagnostic
          diagnosticCount++
          console.info('COMPACTION_HISTORY_SSE_DIAGNOSTIC', JSON.stringify(diagnostic))
        }
        if (category !== 'sse' || message !== 'SSE event rejected' || record(detail).code !== 'sse_event_rejected') return
        const diagnostic = record(detail).diagnostic
        if (!diagnostic || typeof diagnostic !== 'object' || Array.isArray(diagnostic) ||
            !Array.isArray(record(diagnostic).fieldPaths)) return
        const { validation, stage, reason, fieldPaths } = diagnostic as PublicRuntimeSseDecodeDiagnostic
        decodeDiagnostics.push({ validation, stage, reason, fieldPaths: [...fieldPaths] })
      } })
    const bridge: Pick<RuntimeApi, 'runtimeRequest' | 'startSse' | 'ackSseEvent' | 'stopSse' | 'onSseEvent' | 'onSseEnd' | 'onSseError' | 'onRuntimeStatus'> = {
      runtimeRequest: (route, method, body) => runtimeRequestViaHost(settings, route, { method, body }, async () => undefined),
      startSse: async (id, sinceSeq, streamId, replay) => {
        const starting = (async () => {
          const result = await handlers.get('runtime:sse:start')!({ sender }, { threadId: id, sinceSeq, streamId, ...replay }) as { streamId: string }
          streams.add(result.streamId); return result
        })()
        pendingStarts.add(starting)
        try { return await starting } finally { pendingStarts.delete(starting) }
      },
      ackSseEvent: async (streamId, seq, acceptedFinal) => await handlers.get('runtime:sse:ack')!({ sender }, acceptedFinal ? { streamId, seq, ...acceptedFinal } : { streamId, seq }) === true,
      stopSse: async streamId => await handlers.get('runtime:sse:stop')!({ sender }, streamId) === true,
      onSseEvent: fn => { dataListeners.add(fn); return () => { dataListeners.delete(fn) } },
      onSseEnd: fn => { endListeners.add(fn); return () => { endListeners.delete(fn) } },
      onSseError: fn => { errorListeners.add(fn); return () => { errorListeners.delete(fn) } },
      onRuntimeStatus: () => () => undefined
    }
    window.analytix = { settings: { getSettings: async () => settings }, runtime: bridge,
      logs: { error: async () => undefined } } as unknown as Window['analytix']
    rendererRuntimeClient.invalidateSettings(); resetProviderCacheForTests()
    const provider = new AnalytixRuntimeProvider()
    const before = await provider.getThreadDetail(threadId)
    const old = before.blocks.filter(block => block.kind === 'user' || block.kind === 'assistant')
    expect(old.filter(block => block.kind === 'user')).toHaveLength(prompts.length)
    expect(old.filter(block => block.kind === 'assistant')).toHaveLength(prompts.length)
    acceptancePhase = 'compaction'
    await provider.compactThread(threadId)
    const compacted = await provider.getThreadDetail(threadId)
    expect(compacted.blocks.some(block => block.kind === 'compaction')).toBe(true)
    expect(compacted.blocks.some(block => block.id === old[0].id)).toBe(false)
    acceptancePhase = 'restart'
    await stopStreams(); await stopRuntime!(); stopRuntime = undefined
    runtimeUrl = await startRuntime(); settings.runtime!.baseUrl = runtimeUrl; settings.runtime!.port = Number(new URL(runtimeUrl).port)
    rendererRuntimeClient.invalidateSettings(); resetProviderCacheForTests()
    const restartedPrimary = await provider.getThreadDetail(threadId)
    const other = await json('/v1/threads', { workspace: workspaceRoot, title: 'Other ordinary thread', model: 'deepseek-chat' })
    const threads = await provider.listThreads()
    useChatStore.setState({ ...initialState, runtimeConnection: 'ready', threads, workspaceRoot,
      composerModel: 'deepseek-chat', composerProviderId: 'deepseek',
      refreshThreads: async () => undefined, drainQueuedMessages: async () => undefined }, true)
    function TimelineFromStore() {
      const state = useChatStore()
      return createElement(MessageTimeline, { blocks: state.blocks, live: state.liveAssistant, activeThreadId: state.activeThreadId,
        runtimeConnection: state.runtimeConnection, onRetryConnection() {}, onOpenSettings() {} })
    }
    await act(async () => uiRoot.render(createElement(TimelineFromStore)))
    const requestCount = providerRequests.length
    acceptancePhase = 'select_after_restart'
    await act(async () => useChatStore.getState().selectThread(threadId))
    const displayed = useChatStore.getState()
    expect(displayed.error, `Thread selection SSE diagnostics: ${JSON.stringify({ sseErrorDiagnostics, decodeDiagnostics, rejectedSseFrames,
      firstSseFailure: receipt.firstSseFailure })}`).toBeNull()
    expect(displayed.activeThreadId).toBe(threadId)
    expect(displayed.lastSeq).toBe(restartedPrimary.latestSeq)
    acceptancePhase = 'history_interactions'
    expect(displayed.blocks.filter(block => block.kind === 'compaction')).toHaveLength(1)
    const markerIndex = displayed.blocks.findIndex(block => block.kind === 'compaction')
    for (const block of old) {
      const matches = displayed.blocks.filter(candidate => candidate.id === block.id)
      expect(matches).toHaveLength(1)
      expect(matches[0]).toMatchObject({ kind: block.kind, text: 'text' in block ? block.text : undefined })
      expect(displayed.blocks.indexOf(matches[0])).toBeLessThan(markerIndex)
      if (!restartedPrimary.blocks.some(primary => primary.id === block.id)) {
        expect(isOrdinaryDisplayHistoryBlock(matches[0])).toBe(true)
        expect(matches[0]).toHaveProperty('displayHistory', { source: 'host_public_replay', threadId, throughSeq: restartedPrimary.latestSeq })
      }
    }
    // Exercise the existing history page and prepend anchor with measured
    // jsdom geometry; pixel/native browser acceptance remains separate.
    const scroller = container.querySelector<HTMLDivElement>('[data-analytix-message-timeline-scroller]')!
    const earlier = container.querySelector<HTMLButtonElement>('[data-analytix-timeline-row="loadEarlier"] button')
    expect(earlier).not.toBeNull()
    expect(container.textContent).not.toContain(prompts[0])
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, value: 400 })
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true,
      get: () => scroller.querySelectorAll('[data-analytix-timeline-row="turn"]').length * 100 })
    scroller.scrollTop = 250
    await act(async () => scroller.dispatchEvent(new Event('scroll')))
    const previousTop = scroller.scrollTop, previousHeight = scroller.scrollHeight
    await act(async () => { earlier!.click(); await new Promise(resolve => setTimeout(resolve, 25)) })
    expect(scroller.scrollHeight).toBeGreaterThan(previousHeight)
    expect(scroller.scrollTop).toBe(previousTop + scroller.scrollHeight - previousHeight)
    for (const prompt of prompts) expect(container.textContent).toContain(prompt)
    for (const block of old) if (block.kind === 'assistant') expect(container.textContent).toContain(block.text)
    const marker = displayed.blocks.find(block => block.kind === 'compaction')
    expect(marker?.summaryKind).toBe('conversation')
    const summaryText = marker?.detail?.trim()
    expect(summaryText).toBeTruthy()
    const markerButton = container.querySelector<HTMLButtonElement>('[data-analytix-compaction] button')
    expect(markerButton).not.toBeNull()
    expect(markerButton!.disabled).toBe(false)
    expect(markerButton!.getAttribute('aria-expanded')).toBe('false')
    const markerBodyId = markerButton!.getAttribute('aria-controls')
    expect(markerBodyId).toBeTruthy()
    expect(document.getElementById(markerBodyId!)).toBeNull()
    await act(async () => markerButton!.click())
    expect(markerButton!.getAttribute('aria-expanded')).toBe('true')
    expect(document.getElementById(markerBodyId!)?.textContent).toContain(summaryText!)
    await act(async () => markerButton!.click())
    expect(markerButton!.getAttribute('aria-expanded')).toBe('false')
    expect(document.getElementById(markerBodyId!)).toBeNull()
    expect((await provider.getThreadDetail(threadId)).blocks).toEqual(restartedPrimary.blocks)
    acceptancePhase = 'navigation'
    await act(async () => useChatStore.getState().selectThread(String(other.id)))
    await act(async () => useChatStore.getState().selectThread(threadId))
    expect(useChatStore.getState().blocks.map(block => block.id)).toEqual(displayed.blocks.map(block => block.id))
    expect(providerRequests).toHaveLength(requestCount)
    // Send and interrupt through the actual store, with a Provider response
    // held until the runtime cancels it. No old display blocks become prompt.
    acceptancePhase = 'send_after_restore'
    holdNextResponse = true
    await act(async () => {
      expect(await useChatStore.getState().sendMessage('Continue with only the compacted primary context.', 'agent',
        { model: 'deepseek-chat', providerId: 'deepseek' })).toBe(true)
    })
    await waitFor(async () => providerRequests.length === requestCount + 1)
    expect(useChatStore.getState().currentTurnId).toBeTruthy()
    expect(providerRequests.at(-1)).not.toContain(prompts[0])
    expect(providerRequests.at(-1)).not.toContain(prompts[1])
    for (const block of old) {
      if (block.kind !== 'assistant' || restartedPrimary.blocks.some(primary =>
        primary.id === block.id || 'text' in primary && primary.text === block.text)) continue
      expect(providerRequests.at(-1)).not.toContain(block.text)
    }
    acceptancePhase = 'interrupt_after_restore'
    await act(async () => useChatStore.getState().interrupt())
    expect(useChatStore.getState().busy).toBe(false)
    expect(useChatStore.getState().currentTurnId).toBeNull()
    const interruptedPrimary = await provider.getThreadDetail(threadId)
    expect(useChatStore.getState().lastSeq).toBe(interruptedPrimary.latestSeq)
    expect(interruptedPrimary.blocks.some(isOrdinaryDisplayHistoryBlock)).toBe(false)
    // The new turn preserves the existing page size, so open its one earlier
    // turn through the actual Timeline before checking all old visible text.
    const interruptedEarlier = container.querySelector<HTMLButtonElement>('[data-analytix-timeline-row="loadEarlier"] button')
    expect(interruptedEarlier).not.toBeNull()
    await act(async () => { interruptedEarlier!.click(); await new Promise(resolve => setTimeout(resolve, 25)) })
    for (const block of old) {
      const matches = useChatStore.getState().blocks.filter(candidate => candidate.id === block.id)
      expect(matches).toHaveLength(1)
      if ('text' in block) expect(container.textContent).toContain(block.text)
    }
    expect(useChatStore.getState().blocks.some(isOrdinaryDisplayHistoryBlock)).toBe(true)
    acceptanceCompleted = true
  } finally {
    receipt.acceptancePhaseBeforeCleanup = acceptancePhase
    acceptancePhase = 'cleanup'
    if (acceptanceStartedAt) receipt.acceptanceDurationMs = Date.now() - acceptanceStartedAt
    releaseHeldResponse?.()
    await stopStreams(); clearBusyWatchdog(); stopTurnCompletionPoll()
    await act(async () => { uiRoot.unmount(); useChatStore.setState(initialState, true) })
    container.remove(); rendererRuntimeClient.invalidateSettings(); resetProviderCacheForTests()
    // A start may still be loading settings after the first stop sweep.
    while (pendingStarts.size > 0) await Promise.allSettled(Array.from(pendingStarts))
    await stopStreams()
    if (waitForSseWorkers) { await waitForSseWorkers(); receipt.sseWorkersSettled = true }
    vi.unstubAllGlobals()
    if (originalBridge) Object.defineProperty(window, 'analytix', originalBridge)
    else Reflect.deleteProperty(window, 'analytix')
    receipt.sseDiagnosticCount = diagnosticCount
    if (acceptanceCompleted) receipt.acceptancePassed = !workerCrashed
    if (stopRuntime) await stopRuntime()
    await new Promise<void>(resolve => providerServer.close(resolve))
    receipt.providerRequests = providerRequests.length
    receipt.ownedRuntimeProcessesExited = true; receipt.providerListenerClosed = true
    if (buildClosed) {
      if (stateRoot) await fs.rm(stateRoot, { recursive: true, force: true })
      if (workspaceRoot) await fs.rm(workspaceRoot, { recursive: true, force: true })
      if (fixtureRoot) await fs.rm(fixtureRoot, { recursive: true, force: true })
      if (binaryRoot && !receipt.binaryInputsReceiptSha256) await fs.rm(binaryRoot, { recursive: true, force: true })
    }
    receipt.ownedBuildProcessClosed = buildClosed; receipt.ownedFixtureRootsRemoved = buildClosed
    receipt.qualifiedBinaryRetained = Boolean(receipt.binarySha256 && receipt.binaryInputsReceiptSha256)
    if (validationReceiptPath) {
      await fs.writeFile(validationReceiptPath,
        JSON.stringify(receipt, null, 2) + '\n', { mode: 0o600, flag: 'wx' })
    }
    console.info('COMPACTION_HISTORY_CLEANUP_RECEIPT', JSON.stringify(receipt))
    if (acceptanceCompleted) expect(workerCrashed, 'SSE worker crashed before cleanup completed.').toBe(false)
  }
}, 900_000)
