import type { IpcMain, WebContents } from 'electron'
import { randomUUID } from 'node:crypto'
import { URL } from 'node:url'
import type { AppSettingsV1 } from '../shared/app-settings'
import { analytixThreadEventsPath } from '../shared/analytix-endpoints'
import {
  containsPrivateAcceptedFinalAuthority,
  PublicRuntimeEventFilter
} from '../shared/public-runtime-content'
import {
  isPublicRuntimeSseRejectionReasonCode,
  projectPublicRuntimeSseBlock,
  takePublicRuntimeSseBlock,
  type PublicRuntimeSseDecodeDiagnostic
} from '../shared/public-runtime-sse'
import { sseAckPayloadSchema, sseStartPayloadSchema, streamIdSchema } from './ipc/app-ipc-schemas'
import type { JsonSettingsStore } from './settings-store'
import {
  captureCurrentFinalPublicationAuthorityPin,
  getRuntimeBaseUrlForSettings,
  isCurrentFinalPublicationAuthorityPin,
  runtimeAuthHeaders,
  type ManagedFinalPublicationAuthorityPinV1
} from './runtime/analytix-adapter'
import { verifiedAcceptedFinalDeliveryBatch } from './accepted-final-publication'
import { generalTerminalDeliveryBatchVerificationV1 } from './general-terminal-publication'
import { sanitizeThreadTraceEvent, type ThreadTraceEventPayload } from '../shared/thread-trace'

type SseControllerState = {
  controller: AbortController
  owner: WebContents
  ownerId: number
  threadId: string
  stoppedByClient: boolean
  ackedSinceSeq: number
  deliveredSinceSeq: number
  deliveredAckSeqs: Set<number>
  requiredAcceptedFinalAck?: AcceptedFinalAckBinding
  ackWaiters: Set<() => void>
  activeReader?: ReadableStreamDefaultReader<Uint8Array>
  publicEventFilter: PublicRuntimeEventFilter
}

type AcceptedFinalAckBinding = Readonly<{
  seq: number
  batchId: string
  threadId: string
  turnId: string
  publicationCommitId: string
}>

const SSE_RECONNECT_BASE_MS = 750
const SSE_RECONNECT_MAX_MS = 5_000
const SSE_START_TIMEOUT_MS = 15_000
const SSE_ENSURE_RETRY_WINDOW_MS = 45_000
const SSE_PENDING_EVENT_BATCH_MS = 16
const SSE_ACK_GRACE_MS = 500
const SSE_REPLAY_TIMEOUT_MS = 45_000
// An ordinary terminal carrier can contain 8 MiB of text before JSON escaping.
const SSE_REPLAY_MAX_FRAME_CHARS = 64 * 1024 * 1024

const sseControllers = new Map<string, SseControllerState>()
const sseOwnerDestroyedHandlers = new WeakMap<WebContents, () => void>()

function webContentsOwnerId(webContents: WebContents): number {
  const id = webContents.id
  if (!Number.isSafeInteger(id) || id < 0) throw new Error('sse_owner_unavailable')
  return id
}

function deleteControllerIfCurrent(streamId: string, state: SseControllerState): void {
  if (sseControllers.get(streamId) === state) sseControllers.delete(streamId)
}

function stopSseController(state: SseControllerState): void {
  state.stoppedByClient = true
  state.controller.abort()
  const reader = state.activeReader
  if (reader) void reader.cancel().catch(() => undefined)
}

function ensureSseOwnerDestroyedObserver(owner: WebContents): void {
  if (sseOwnerDestroyedHandlers.has(owner)) return
  const emitter = owner as WebContents & {
    once?: (event: 'destroyed', listener: () => void) => unknown
  }
  if (typeof emitter.once !== 'function') return
  const onDestroyed = (): void => {
    for (const [streamId, state] of Array.from(sseControllers.entries())) {
      if (state.owner !== owner) continue
      stopSseController(state)
      deleteControllerIfCurrent(streamId, state)
    }
    sseOwnerDestroyedHandlers.delete(owner)
  }
  sseOwnerDestroyedHandlers.set(owner, onDestroyed)
  emitter.once('destroyed', onDestroyed)
}

function sendSseMessage(wc: WebContents, channel: string, payload: unknown): boolean {
  if (wc.isDestroyed()) return false
  try {
    wc.send(channel, payload)
    return true
  } catch {
    return false
  }
}

async function sleepWithAbort(ms: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted || ms <= 0) return
  await new Promise<void>((resolve) => {
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    const onAbort = (): void => {
      clearTimeout(timer)
      signal.removeEventListener('abort', onAbort)
      resolve()
    }
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

async function waitForSseAck(
  state: SseControllerState,
  seq: number,
  signal: AbortSignal,
  timeoutMs: number | null = SSE_ACK_GRACE_MS
): Promise<void> {
  if (seq <= 0 || state.ackedSinceSeq >= seq || signal.aborted || state.stoppedByClient) return
  await new Promise<void>((resolve) => {
    let timer: ReturnType<typeof setTimeout> | undefined
    const done = (): void => {
      if (timer) clearTimeout(timer)
      state.ackWaiters.delete(onAck)
      signal.removeEventListener('abort', done)
      resolve()
    }
    const onAck = (): void => {
      if (state.ackedSinceSeq >= seq) done()
    }
    if (timeoutMs !== null) timer = setTimeout(done, timeoutMs)
    state.ackWaiters.add(onAck)
    signal.addEventListener('abort', done, { once: true })
  })
}

function isTerminalRuntimeSseEvent(event: Record<string, unknown>): boolean {
  return event.kind === 'accepted_final_batch' || event.kind === 'general_terminal_batch' ||
    event.kind === 'public_projection_revoked' ||
    (event.kind === 'error' && (event.terminal === true || event.fatal === true))
}

function claimsAcceptedFinalAuthority(event: Record<string, unknown>): boolean {
  const item = event.item && typeof event.item === 'object' && !Array.isArray(event.item)
    ? event.item as Record<string, unknown>
    : null
  return containsPrivateAcceptedFinalAuthority(event) || [
    'acceptedFinalDigest',
    'publicationCommitId',
    'publicationEventId',
    'publicationSlot',
    'publicationPayloadDigest'
  ].some((key) => Object.prototype.hasOwnProperty.call(event, key)) ||
    Boolean(item && (
      Object.prototype.hasOwnProperty.call(item, 'acceptedFinal') ||
      Object.prototype.hasOwnProperty.call(item, 'acceptedFinalView')
    ))
}

function acceptedFinalAuthorityIsValid(event: Record<string, unknown>): boolean {
  // Accepted-final authority is released only through one verified delivery
  // batch. A standalone signed item is still a prefix and must fail closed.
  return !claimsAcceptedFinalAuthority(event)
}

function isFatalSseStatus(status: number | undefined): boolean {
  return typeof status === 'number' && status >= 400 && status < 500 && status !== 408 && status !== 429
}

function runtimeEnsureErrorPayload(error: unknown): { code: string; message: string } {
  const rawMessage = error instanceof Error ? error.message : String(error)
  try {
    const parsed = JSON.parse(rawMessage) as { code?: unknown; message?: unknown }
    return {
      code: typeof parsed.code === 'string' ? parsed.code : '',
      message: typeof parsed.message === 'string' ? parsed.message : rawMessage
    }
  } catch {
    return { code: '', message: rawMessage }
  }
}

function publicSseError(code: string, fallbackCode: string): { code: string; message: string } {
  const normalized = /^[A-Za-z0-9_.-]+$/.test(code.trim()) ? code.trim() : fallbackCode
  return { code: normalized, message: `Runtime request failed (${normalized}).` }
}

function isRecoverableRuntimeEnsureError(error: unknown): boolean {
  const payload = runtimeEnsureErrorPayload(error)
  const code = payload.code.toLowerCase()
  const message = payload.message.toLowerCase()
  if (
    code === 'missing_api_key' ||
    code === 'runtime_auth_required' ||
    code === 'invalid_settings' ||
    code === 'provider_request_failed' ||
    code === 'provider_request_error'
  ) {
    return false
  }
  return code === 'fetch_failed' ||
    code === 'runtime_unavailable' ||
    code === 'runtime_unhealthy' ||
    code === 'timeout' ||
    code === 'terminated' ||
    /timeout|timed out|fetch failed|network|econnrefused|terminated|socket|health/.test(message)
}

async function fetchSseWithStartTimeout(
  url: URL,
  headers: Record<string, string>,
  signal: AbortSignal,
  timeoutMs: number
): Promise<Response> {
  const attempt = new AbortController()
  let timedOut = false
  const timer = setTimeout(() => {
    timedOut = true
    attempt.abort()
  }, timeoutMs)
  const onAbort = (): void => {
    attempt.abort()
  }
  signal.addEventListener('abort', onAbort, { once: true })
  try {
    return await fetch(url, { signal: attempt.signal, headers })
  } catch (error) {
    if (timedOut) {
      throw new Error('sse start timeout')
    }
    throw error
  } finally {
    clearTimeout(timer)
    signal.removeEventListener('abort', onAbort)
  }
}

export function registerRuntimeSseIpc(options: {
  ipcMain: IpcMain
  store: JsonSettingsStore
  ensureRuntime: (settings: AppSettingsV1) => Promise<AppSettingsV1 | void>
  logError: (category: string, message: string, detail?: unknown) => void
  resolveFinalPublicationAuthorityPin?: () => ManagedFinalPublicationAuthorityPinV1 | null
  isFinalPublicationAuthorityPinCurrent?: (pin: ManagedFinalPublicationAuthorityPinV1 | null) => boolean
  recordThreadTrace?: (event: ThreadTraceEventPayload) => void
}): void {
  const { ipcMain, store, ensureRuntime, logError } = options
  const resolveAuthorityPin = options.resolveFinalPublicationAuthorityPin ?? captureCurrentFinalPublicationAuthorityPin
  const authorityPinIsCurrent = options.isFinalPublicationAuthorityPinCurrent ?? isCurrentFinalPublicationAuthorityPin
  const recordTerminalTrace = (
    name: 'thread.terminal.verified' | 'thread.terminal.ipc_sent',
    threadId: string,
    lastSeq: number,
    acceptedFinal: boolean,
    verificationMs?: number,
    turnId?: string
  ): void => {
    try {
      options.recordThreadTrace?.(sanitizeThreadTraceEvent({
        name, timestamp: Date.now(), threadId, turnId,
        data: { lastSeq, acceptedFinal, monotonicMs: performance.now(), timeOrigin: performance.timeOrigin, ...(verificationMs === undefined ? {} : { verificationMs }) }
      }))
    } catch {
      // Optional diagnostics cannot interrupt verified delivery or its ACK.
    }
  }
  ipcMain.handle('runtime:sse:start', async (event, args: unknown) => {
    const request = sseStartPayloadSchema.parse(args)
    const replayThroughSeq = request.replayThroughSeq
    const replay = replayThroughSeq !== undefined
    const loadedSettings = await store.load()
    const ownerId = webContentsOwnerId(event.sender)
    const requestedId = request.streamId?.trim() ?? ''
    const id = requestedId || randomUUID()
    const existing = sseControllers.get(id)
    if (existing) {
      if (existing.owner !== event.sender || existing.ownerId !== ownerId) {
        throw new Error('sse_stream_owner_mismatch')
      }
      stopSseController(existing)
      deleteControllerIfCurrent(id, existing)
    }
    const ac = new AbortController()
    const state: SseControllerState = {
      controller: ac,
      owner: event.sender,
      ownerId,
      threadId: request.threadId,
      stoppedByClient: false,
      ackedSinceSeq: request.sinceSeq,
      deliveredSinceSeq: request.sinceSeq,
      deliveredAckSeqs: new Set(),
      ackWaiters: new Set(),
      publicEventFilter: new PublicRuntimeEventFilter()
    }
    sseControllers.set(id, state)
    ensureSseOwnerDestroyedObserver(event.sender)
    const replayTimeout = replay ? setTimeout(() => {
      if (sseControllers.get(id) !== state || ac.signal.aborted) return
      sendSseMessage(event.sender, 'runtime:sse-error', {
        streamId: id, ...publicSseError('', 'sse_stream_error')
      })
      stopSseController(state)
      deleteControllerIfCurrent(id, state)
    }, SSE_REPLAY_TIMEOUT_MS) : undefined

    ;(async () => {
      const wc = event.sender
      let s = loadedSettings
      let reconnectDelayMs = SSE_RECONNECT_BASE_MS
      const ensureDeadline = Date.now() + SSE_ENSURE_RETRY_WINDOW_MS
      while (!state.stoppedByClient && !ac.signal.aborted) {
        try {
          const ensuredSettings = await ensureRuntime(s)
          s = ensuredSettings ?? s
          break
        } catch (error) {
          const payload = runtimeEnsureErrorPayload(error)
          const canRetry = isRecoverableRuntimeEnsureError(error) && Date.now() < ensureDeadline
          if (!canRetry) {
            const publicError = publicSseError(payload.code, 'sse_setup_error')
            if (!sendSseMessage(wc, 'runtime:sse-error', {
              streamId: id,
              ...publicError
            })) {
              stopSseController(state)
            }
            logError('sse', 'SSE runtime ensure failed', {
              code: publicError.code
            })
            deleteControllerIfCurrent(id, state)
            return
          }
          await sleepWithAbort(reconnectDelayMs, ac.signal)
          reconnectDelayMs = Math.min(reconnectDelayMs * 2, SSE_RECONNECT_MAX_MS)
        }
      }
      if (state.stoppedByClient || ac.signal.aborted) {
        deleteControllerIfCurrent(id, state)
        return
      }

      reconnectDelayMs = SSE_RECONNECT_BASE_MS
      let connectionAttempt = 0
      try {
        while (!state.stoppedByClient && !ac.signal.aborted) {
          if (connectionAttempt > 0) {
            const reconnectEnsureDeadline = Date.now() + SSE_ENSURE_RETRY_WINDOW_MS
            while (!state.stoppedByClient && !ac.signal.aborted) {
              try {
                const latestSettings = await store.load()
                const ensuredSettings = await ensureRuntime(latestSettings)
                s = ensuredSettings ?? latestSettings
                break
              } catch (error) {
                const payload = runtimeEnsureErrorPayload(error)
                const canRetry = isRecoverableRuntimeEnsureError(error) &&
                  Date.now() < reconnectEnsureDeadline
                if (!canRetry) {
                  const publicError = publicSseError(payload.code, 'sse_setup_error')
                  if (!sendSseMessage(wc, 'runtime:sse-error', {
                    streamId: id,
                    ...publicError
                  })) {
                    stopSseController(state)
                  }
                  logError('sse', 'SSE runtime ensure failed', {
                    code: publicError.code,
                    phase: 'reconnect'
                  })
                  return
                }
                await sleepWithAbort(reconnectDelayMs, ac.signal)
                reconnectDelayMs = Math.min(reconnectDelayMs * 2, SSE_RECONNECT_MAX_MS)
              }
            }
            if (state.stoppedByClient || ac.signal.aborted) return
          }
          connectionAttempt += 1
          const base = getRuntimeBaseUrlForSettings(s)
          const capturedAuthorityPin = resolveAuthorityPin()
          const connectionAuthorityPin = capturedAuthorityPin &&
            new URL(base).origin === capturedAuthorityPin.runtimeUrl
            ? capturedAuthorityPin
            : null
          const connectionAuthorityPinUnavailableReason = capturedAuthorityPin
            ? 'accepted_final_authority_pin_origin_mismatch'
            : 'accepted_final_authority_pin_unavailable'
          const headers: Record<string, string> = { Accept: 'text/event-stream' }
          runtimeAuthHeaders(s).forEach((value, key) => {
            headers[key] = value
          })
          const url = new URL(`${base}${analytixThreadEventsPath(request.threadId)}`)
          url.searchParams.set('since_seq', String(state.ackedSinceSeq))
          if (!replay) url.searchParams.set('live', '1')
          const requestHeaders = { ...headers }
          if (state.ackedSinceSeq > 0) {
            requestHeaders['Last-Event-ID'] = String(state.ackedSinceSeq)
          } else {
            delete requestHeaders['Last-Event-ID']
          }
          try {
            const res = await fetchSseWithStartTimeout(url, requestHeaders, ac.signal, SSE_START_TIMEOUT_MS)
            if (state.stoppedByClient || ac.signal.aborted) {
              void res.body?.cancel().catch(() => undefined)
              return
            }
            if (!res.ok || !res.body) {
              if (replay || isFatalSseStatus(res.status)) {
                if (!sendSseMessage(wc, 'runtime:sse-error', { streamId: id, status: res.status })) {
                  stopSseController(state)
                }
                logError('sse', 'SSE connection failed', {
                  statusCode: res.status
                })
                return
              }
              await sleepWithAbort(reconnectDelayMs, ac.signal)
              reconnectDelayMs = Math.min(reconnectDelayMs * 2, SSE_RECONNECT_MAX_MS)
              continue
            }
            reconnectDelayMs = SSE_RECONNECT_BASE_MS
            const reader = res.body.getReader()
            state.activeReader = reader
            const dec = new TextDecoder()
            let buffer = ''

            let pendingEvents: Record<string, unknown>[] = []
            let throttleTimer: ReturnType<typeof setTimeout> | undefined
            let firstEventFlushed = false
            let terminalRuntimeEventSeen = false
            let terminalAckRequired = false
            let protocolViolationReason = ''
            let protocolViolationDiagnostic: PublicRuntimeSseDecodeDiagnostic | undefined
            const recordDecoderDiagnostic = (diagnostic: PublicRuntimeSseDecodeDiagnostic): void => {
              protocolViolationDiagnostic ??= diagnostic
            }
            let refreshConnectionAuthority = false
            let replayBoundaryReached = false
            const replayAuthorityIsCurrent = (): boolean => !replay || !connectionAuthorityPin ||
              authorityPinIsCurrent(connectionAuthorityPin)

            const flushEvents = (): boolean => {
              if (throttleTimer) {
                clearTimeout(throttleTimer)
                throttleTimer = undefined
              }
              if (state.stoppedByClient || ac.signal.aborted) {
                pendingEvents = []
                return false
              }
              if (pendingEvents.length === 0) return true
              if (!replayAuthorityIsCurrent()) {
                protocolViolationReason = 'invalid_public_projection'
                pendingEvents = []
                return false
              }

              let batchMaxSeq = state.deliveredSinceSeq
              for (const event of pendingEvents) {
                if (typeof event.seq === 'number') {
                  batchMaxSeq = Math.max(batchMaxSeq, event.seq)
                }
                if (isTerminalRuntimeSseEvent(event) && (!replay || event.kind !== 'general_terminal_batch')) {
                  terminalRuntimeEventSeen = true
                  if (event.kind === 'accepted_final_batch') terminalAckRequired = true
                }
              }
              const batch = pendingEvents
              pendingEvents = []
              if (!sendSseMessage(wc, 'runtime:sse-event', { streamId: id, events: batch })) {
                stopSseController(state)
                return false
              }
              for (const delivered of batch) {
                if (delivered.kind !== 'accepted_final_batch' && delivered.kind !== 'general_terminal_batch') continue
                recordTerminalTrace('thread.terminal.ipc_sent', state.threadId, batchMaxSeq,
                  delivered.kind === 'accepted_final_batch', undefined,
                  typeof delivered.turnId === 'string' ? delivered.turnId : undefined)
              }
              state.deliveredSinceSeq = batchMaxSeq
              state.deliveredAckSeqs.add(batchMaxSeq)
              if (terminalRuntimeEventSeen) return false
              return true
            }

            const admitEvent = (event: Record<string, unknown>): boolean => {
              if (replay && event.kind === 'error' && (event.terminal === true || event.fatal === true || event.code === 'sse_setup_error')) {
                protocolViolationReason = 'invalid_public_projection'
                return false
              }
              if (event.kind === 'accepted_final_batch') {
                if (replay) { protocolViolationReason = 'invalid_public_projection'; return false }
                const verificationStartedAt = performance.now()
                if (!connectionAuthorityPin) {
                  protocolViolationReason = connectionAuthorityPinUnavailableReason
                  return false
                }
                if (!authorityPinIsCurrent(connectionAuthorityPin)) {
                  refreshConnectionAuthority = true
                  return false
                }
                const accepted = verifiedAcceptedFinalDeliveryBatch(
                  event,
                  connectionAuthorityPin
                )
                if (!accepted) {
                  protocolViolationReason = 'accepted_final_batch_invalid'
                  return false
                }
                recordTerminalTrace('thread.terminal.verified', state.threadId, accepted.lastSeq, true,
                  performance.now() - verificationStartedAt, accepted.batch.turnId as string)
                state.requiredAcceptedFinalAck = {
                  seq: accepted.lastSeq,
                  batchId: accepted.batch.batchId as string,
                  threadId: accepted.batch.threadId as string,
                  turnId: accepted.batch.turnId as string,
                  publicationCommitId: accepted.publicationCommitId
                }
                if (pendingEvents.length > 0 && !flushEvents()) return false
                pendingEvents.push(accepted.batch)
                flushEvents()
                return false
              }
              if (event.kind === 'general_terminal_batch') {
                const verificationStartedAt = performance.now()
                const verification = generalTerminalDeliveryBatchVerificationV1(event)
                const verified = verification.verified
                if (!verified) {
                  protocolViolationReason = verification.reason ??
                    'general_terminal_batch_verification_failed'
                  return false
                }
                recordTerminalTrace('thread.terminal.verified', state.threadId, verified.lastSeq, false,
                  performance.now() - verificationStartedAt, verified.batch.turnId as string)
                // Ordinary terminal delivery is also an indivisible transport
                // unit. It has no evidence/fact authority, but mixing it with
                // progress would make the preload reject the whole IPC payload
                // after the main process had already advanced its cursor.
                if (pendingEvents.length > 0 && !flushEvents()) return false
                pendingEvents.push(verified.batch)
                terminalAckRequired = true
                flushEvents()
                return replay
              }
              if (!acceptedFinalAuthorityIsValid(event)) {
                protocolViolationReason = 'accepted_final_authority_invalid'
                return false
              }
              pendingEvents.push(event)
              return true
            }

            try {
              while (true) {
                const { done, value } = await reader.read()
                if (state.stoppedByClient || ac.signal.aborted) break
                if (done) break
                buffer += dec.decode(value, { stream: true })
                let next: { block: string; rest: string } | null
                let hasNewEvents = false
                while ((next = takePublicRuntimeSseBlock(buffer)) !== null) {
                  const block = next.block
                  buffer = next.rest
                  if (replay && block.length > SSE_REPLAY_MAX_FRAME_CHARS) {
                    protocolViolationReason = 'malformed_frame'
                    break
                  }
                  const decision = projectPublicRuntimeSseBlock(
                    block, state.threadId, state.publicEventFilter, recordDecoderDiagnostic
                  )
                  if (decision === null) continue
                  if (decision.status === 'revoke') {
                    // Revocation is a terminal, non-durable control. Never
                    // mix it with queued events or advance their cursor first.
                    pendingEvents = [decision.event]
                    terminalRuntimeEventSeen = true
                    flushEvents()
                    break
                  }
                  if (decision.status !== 'emit') {
                    protocolViolationReason = decision.reason
                    break
                  }
                  if (replay) {
                    // Setup errors use the requested cursor (including zero),
                    // so they must fail before normal replay deduplication.
                    if (decision.event.kind === 'error' && (decision.event.terminal === true ||
                        decision.event.fatal === true || decision.event.code === 'sse_setup_error')) {
                      protocolViolationReason = 'invalid_public_projection'
                      break
                    }
                    if (decision.seq > replayThroughSeq!) {
                      if (typeof decision.event.firstSeq === 'number' && decision.event.firstSeq <= replayThroughSeq!) {
                        protocolViolationReason = 'invalid_public_projection'
                      } else {
                        replayBoundaryReached = true
                      }
                      break
                    }
                    if (decision.seq <= state.ackedSinceSeq) continue
                    if (typeof decision.event.firstSeq === 'number' && decision.event.firstSeq <= state.ackedSinceSeq) {
                      protocolViolationReason = 'invalid_public_projection'
                      break
                    }
                  }
                  if (!admitEvent(decision.event)) break
                  if (replay) {
                    if (!flushEvents()) break
                    await waitForSseAck(state, state.deliveredSinceSeq, ac.signal, null)
                    if (state.stoppedByClient || ac.signal.aborted) break
                    if (!replayAuthorityIsCurrent()) {
                      protocolViolationReason = 'invalid_public_projection'
                      break
                    }
                  } else if (!firstEventFlushed) {
                    firstEventFlushed = true
                    if (!flushEvents()) break
                  } else {
                    hasNewEvents = true
                  }
                }
                if (state.stoppedByClient || ac.signal.aborted || terminalRuntimeEventSeen || protocolViolationReason || refreshConnectionAuthority || replayBoundaryReached) break
                if (replay && buffer.length > SSE_REPLAY_MAX_FRAME_CHARS) {
                  protocolViolationReason = 'malformed_frame'
                  break
                }
                if (hasNewEvents && !throttleTimer) {
                  throttleTimer = setTimeout(() => {
                    throttleTimer = undefined
                    flushEvents()
                  }, SSE_PENDING_EVENT_BATCH_MS)
                }
              }
              buffer += dec.decode()
              if (replay && !replayBoundaryReached && !state.stoppedByClient && !ac.signal.aborted &&
                  buffer.length > SSE_REPLAY_MAX_FRAME_CHARS) protocolViolationReason = 'malformed_frame'
              // A delivered terminal ends this connection. Coalesced later history
              // belongs to a subsequent replay, not to an incomplete final frame.
              if (!state.stoppedByClient && !ac.signal.aborted && !terminalRuntimeEventSeen && !replayBoundaryReached &&
                  buffer.trim() && !protocolViolationReason && !refreshConnectionAuthority) {
                const decision = projectPublicRuntimeSseBlock(
                  buffer, state.threadId, state.publicEventFilter, recordDecoderDiagnostic
                )
                if (decision?.status === 'emit') {
                  if (replay && decision.event.kind === 'error' && (decision.event.terminal === true ||
                      decision.event.fatal === true || decision.event.code === 'sse_setup_error')) {
                    protocolViolationReason = 'invalid_public_projection'
                  } else if (replay && decision.seq > replayThroughSeq!) {
                    if (typeof decision.event.firstSeq === 'number' && decision.event.firstSeq <= replayThroughSeq!) {
                      protocolViolationReason = 'invalid_public_projection'
                    }
                  } else if (replay && decision.seq > state.ackedSinceSeq &&
                      typeof decision.event.firstSeq === 'number' && decision.event.firstSeq <= state.ackedSinceSeq) {
                    protocolViolationReason = 'invalid_public_projection'
                  } else if (!replay || decision.seq > state.ackedSinceSeq) {
                    admitEvent(decision.event)
                  }
                } else if (decision?.status === 'revoke') {
                  pendingEvents = [decision.event]
                  terminalRuntimeEventSeen = true
                } else if (decision !== null) {
                  protocolViolationReason = decision?.reason ?? 'malformed_frame'
                }
              }
            } finally {
              if (throttleTimer) {
                clearTimeout(throttleTimer)
                throttleTimer = undefined
              }
              if (refreshConnectionAuthority) {
                pendingEvents = []
              } else {
                flushEvents()
              }
              try {
                await reader.cancel()
              } catch {
                // Cancellation may race stop/owner destruction or natural EOF.
              }
              if (state.activeReader === reader) state.activeReader = undefined
              try {
                reader.releaseLock()
              } catch {
                // A concurrently cancelled reader may already be released.
              }
            }
            if (refreshConnectionAuthority) {
              if (replay) throw new Error('replay authority changed')
              continue
            }
            if (protocolViolationReason) {
              try {
                await reader.cancel()
              } catch {
                // The stream may already be closed; rejection remains fail-closed.
              }
              const publicError = publicSseError('', 'sse_event_rejected')
              const reasonCode = isPublicRuntimeSseRejectionReasonCode(protocolViolationReason)
                ? protocolViolationReason
                : 'invalid_public_projection'
              sendSseMessage(wc, 'runtime:sse-error', {
                streamId: id,
                ...publicError,
                reasonCode
              })
              logError('sse', 'SSE event rejected', {
                code: publicError.code,
                reasonCode,
                ...(protocolViolationDiagnostic ? { diagnostic: protocolViolationDiagnostic } : {})
              })
              if (protocolViolationDiagnostic && process.env.ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK === '1') {
                console.error('[packaged-sse-schema] ' + JSON.stringify({ reasonCode, diagnostic: protocolViolationDiagnostic }))
              }
              return
            }
            if (terminalRuntimeEventSeen && terminalAckRequired) {
              await waitForSseAck(state, state.deliveredSinceSeq, ac.signal, null)
            }
            if (terminalRuntimeEventSeen) {
              return
            }
            await waitForSseAck(state, state.deliveredSinceSeq, ac.signal, replay ? null : SSE_ACK_GRACE_MS)
            if (replay) {
              if (!state.stoppedByClient && !ac.signal.aborted && !replayAuthorityIsCurrent()) {
                throw new Error('replay authority changed')
              }
              return
            }
          } catch (e) {
            if (state.stoppedByClient || ac.signal.aborted) return
            const rawMsg = e instanceof Error ? e.message : String(e)
            if (!replay && (/sse start timeout/i.test(rawMsg) || /fetch failed/i.test(rawMsg) || /network/i.test(rawMsg))) {
              await sleepWithAbort(reconnectDelayMs, ac.signal)
              reconnectDelayMs = Math.min(reconnectDelayMs * 2, SSE_RECONNECT_MAX_MS)
              continue
            }
            const publicError = publicSseError('', 'sse_stream_error')
            if (!sendSseMessage(wc, 'runtime:sse-error', { streamId: id, ...publicError })) {
              stopSseController(state)
            }
            logError('sse', 'SSE stream error', { code: publicError.code })
            return
          }
        }
      } finally {
        if (!state.stoppedByClient && !ac.signal.aborted) {
          sendSseMessage(wc, 'runtime:sse-end', { streamId: id })
        }
        deleteControllerIfCurrent(id, state)
      }
    })().catch(() => {
      if (replay && !state.stoppedByClient && !ac.signal.aborted) {
        sendSseMessage(event.sender, 'runtime:sse-error', { streamId: id, ...publicSseError('', 'sse_stream_error') })
        stopSseController(state)
      }
      deleteControllerIfCurrent(id, state)
      logError('sse', 'SSE worker crashed', {
        code: 'sse_worker_error'
      })
    }).finally(() => {
      if (replayTimeout !== undefined) clearTimeout(replayTimeout)
    })

    return { streamId: id }
  })

  ipcMain.handle('runtime:sse:stop', async (event, streamId: unknown) => {
    const normalizedStreamId = streamIdSchema.parse(streamId)
    const state = sseControllers.get(normalizedStreamId)
    if (!state || state.owner !== event.sender || state.ownerId !== webContentsOwnerId(event.sender)) return false
    stopSseController(state)
    deleteControllerIfCurrent(normalizedStreamId, state)
    return true
  })

  ipcMain.handle('runtime:sse:ack', async (event, args: unknown) => {
    const rejectAck = (reasonCode: 'invalid_ack_request' | 'stream_unavailable' | 'owner_mismatch' |
      'undelivered_sequence' | 'accepted_final_receipt_mismatch' | 'accepted_final_sequence_invalid' |
      'unexpected_accepted_final_receipt'): false => {
      try { logError('sse', 'SSE acknowledgement rejected', { code: 'sse_ack_rejected', reasonCode }) }
      catch { /* Diagnostics must not change the ACK contract. */ }
      return false
    }
    let ack: ReturnType<typeof sseAckPayloadSchema.parse>
    try { ack = sseAckPayloadSchema.parse(args) }
    catch (error) { rejectAck('invalid_ack_request'); throw error }
    const state = sseControllers.get(ack.streamId)
    if (!state) return rejectAck('stream_unavailable')
    if (state.owner !== event.sender || state.ownerId !== webContentsOwnerId(event.sender)) {
      return rejectAck('owner_mismatch')
    }
    if (ack.seq !== state.ackedSinceSeq && !state.deliveredAckSeqs.has(ack.seq)) {
      return rejectAck('undelivered_sequence')
    }
    const acceptedFinalAck = 'batchId' in ack ? ack : null
    const required = state.requiredAcceptedFinalAck
    if (required) {
      if (ack.seq === required.seq) {
        if (!acceptedFinalAck || acceptedFinalAck.batchId !== required.batchId ||
            acceptedFinalAck.threadId !== required.threadId || acceptedFinalAck.turnId !== required.turnId ||
            acceptedFinalAck.publicationCommitId !== required.publicationCommitId) {
          return rejectAck('accepted_final_receipt_mismatch')
        }
      } else if (acceptedFinalAck || ack.seq > required.seq) {
        return rejectAck('accepted_final_sequence_invalid')
      }
    } else if (acceptedFinalAck) {
      return rejectAck('unexpected_accepted_final_receipt')
    }
    state.ackedSinceSeq = Math.max(state.ackedSinceSeq, ack.seq)
    if (required && ack.seq === required.seq) state.requiredAcceptedFinalAck = undefined
    for (const boundary of state.deliveredAckSeqs) {
      if (boundary <= state.ackedSinceSeq) state.deliveredAckSeqs.delete(boundary)
    }
    for (const notify of state.ackWaiters) notify()
    return true
  })
}
