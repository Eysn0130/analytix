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
  takePublicRuntimeSseBlock
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
}): void {
  const { ipcMain, store, ensureRuntime, logError } = options
  const resolveAuthorityPin = options.resolveFinalPublicationAuthorityPin ?? captureCurrentFinalPublicationAuthorityPin
  const authorityPinIsCurrent = options.isFinalPublicationAuthorityPinCurrent ?? isCurrentFinalPublicationAuthorityPin
  ipcMain.handle('runtime:sse:start', async (event, args: unknown) => {
    const request = sseStartPayloadSchema.parse(args)
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
          url.searchParams.set('live', '1')
          const requestHeaders = { ...headers }
          if (state.ackedSinceSeq > 0) {
            requestHeaders['Last-Event-ID'] = String(state.ackedSinceSeq)
          } else {
            delete requestHeaders['Last-Event-ID']
          }
          try {
            const res = await fetchSseWithStartTimeout(url, requestHeaders, ac.signal, SSE_START_TIMEOUT_MS)
            if (!res.ok || !res.body) {
              if (isFatalSseStatus(res.status)) {
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
            let refreshConnectionAuthority = false

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

              let batchMaxSeq = state.deliveredSinceSeq
              for (const event of pendingEvents) {
                if (typeof event.seq === 'number') {
                  batchMaxSeq = Math.max(batchMaxSeq, event.seq)
                }
                if (isTerminalRuntimeSseEvent(event)) {
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
              state.deliveredSinceSeq = batchMaxSeq
              state.deliveredAckSeqs.add(batchMaxSeq)
              if (terminalRuntimeEventSeen) return false
              return true
            }

            const admitEvent = (event: Record<string, unknown>): boolean => {
              if (event.kind === 'accepted_final_batch') {
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
                const verification = generalTerminalDeliveryBatchVerificationV1(event)
                const verified = verification.verified
                if (!verified) {
                  protocolViolationReason = verification.reason ??
                    'general_terminal_batch_verification_failed'
                  return false
                }
                // Ordinary terminal delivery is also an indivisible transport
                // unit. It has no evidence/fact authority, but mixing it with
                // progress would make the preload reject the whole IPC payload
                // after the main process had already advanced its cursor.
                if (pendingEvents.length > 0 && !flushEvents()) return false
                pendingEvents.push(verified.batch)
                terminalAckRequired = true
                flushEvents()
                return false
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
                if (done) break
                buffer += dec.decode(value, { stream: true })
                let next: { block: string; rest: string } | null
                let hasNewEvents = false
                while ((next = takePublicRuntimeSseBlock(buffer)) !== null) {
                  const block = next.block
                  buffer = next.rest
                  const decision = projectPublicRuntimeSseBlock(block, state.threadId, state.publicEventFilter)
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
                  if (!admitEvent(decision.event)) break
                  if (!firstEventFlushed) {
                    firstEventFlushed = true
                    if (!flushEvents()) break
                  } else {
                    hasNewEvents = true
                  }
                }
                if (terminalRuntimeEventSeen || protocolViolationReason || refreshConnectionAuthority) break
                if (hasNewEvents && !throttleTimer) {
                  throttleTimer = setTimeout(() => {
                    throttleTimer = undefined
                    flushEvents()
                  }, SSE_PENDING_EVENT_BATCH_MS)
                }
              }
              buffer += dec.decode()
              if (buffer.trim() && !protocolViolationReason && !refreshConnectionAuthority) {
                const decision = projectPublicRuntimeSseBlock(buffer, state.threadId, state.publicEventFilter)
                if (decision?.status === 'emit') {
                  admitEvent(decision.event)
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
                reasonCode
              })
              return
            }
            if (terminalRuntimeEventSeen && terminalAckRequired) {
              await waitForSseAck(state, state.deliveredSinceSeq, ac.signal, null)
            }
            if (terminalRuntimeEventSeen) {
              return
            }
            await waitForSseAck(state, state.deliveredSinceSeq, ac.signal)
          } catch (e) {
            if (state.stoppedByClient || ac.signal.aborted) return
            const rawMsg = e instanceof Error ? e.message : String(e)
            if (/sse start timeout/i.test(rawMsg) || /fetch failed/i.test(rawMsg) || /network/i.test(rawMsg)) {
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
      deleteControllerIfCurrent(id, state)
      logError('sse', 'SSE worker crashed', {
        code: 'sse_worker_error'
      })
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
    const ack = sseAckPayloadSchema.parse(args)
    const state = sseControllers.get(ack.streamId)
    if (!state || state.owner !== event.sender || state.ownerId !== webContentsOwnerId(event.sender) ||
        (ack.seq !== state.ackedSinceSeq && !state.deliveredAckSeqs.has(ack.seq))) {
      return false
    }
    const acceptedFinalAck = 'batchId' in ack ? ack : null
    const required = state.requiredAcceptedFinalAck
    if (required) {
      if (ack.seq === required.seq) {
        if (!acceptedFinalAck || acceptedFinalAck.batchId !== required.batchId ||
            acceptedFinalAck.threadId !== required.threadId || acceptedFinalAck.turnId !== required.turnId ||
            acceptedFinalAck.publicationCommitId !== required.publicationCommitId) {
          return false
        }
      } else if (acceptedFinalAck || ack.seq > required.seq) {
        return false
      }
    } else if (acceptedFinalAck) {
      return false
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
