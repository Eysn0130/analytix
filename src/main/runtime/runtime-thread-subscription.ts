import { PublicRuntimeEventFilter } from '../../shared/public-runtime-content'
import { projectPublicRuntimeSseBlock, takePublicRuntimeSseBlock } from '../../shared/public-runtime-sse'
import type { RuntimeSseEvent } from '../claw-runtime-helpers'

type SubscriptionOptions = {
  baseUrl: string
  threadId: string
  headers: Record<string, string>
  onEvent: (event: RuntimeSseEvent) => void
  signal: AbortSignal
  logError?: (category: string, message: string, detail?: unknown) => void
}

/** Own one connection, reader and retry at a time; projection remains core-owned. */
export async function startRuntimeThreadSubscription(options: SubscriptionOptions): Promise<{ close: () => void }> {
  const endpoint = new URL(`${options.baseUrl.replace(/\/+$/, '')}/v1/threads/${encodeURIComponent(options.threadId)}/events`)
  const abort = new AbortController()
  const filter = new PublicRuntimeEventFilter()
  let cursor = 0
  let stopped = false
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  let wake: (() => void) | undefined

  const report = (message: string, detail: unknown): void => {
    try { options.logError?.('sse', message, detail) } catch { /* Diagnostics cannot own the connection. */ }
  }
  const close = (): void => {
    if (stopped) return
    stopped = true
    options.signal.removeEventListener('abort', close)
    abort.abort()
    if (timer !== undefined) clearTimeout(timer)
    timer = undefined
    wake?.()
    wake = undefined
    void reader?.cancel().catch(() => undefined)
  }
  const emitTerminal = (event: RuntimeSseEvent, callbackFailure: string): void => {
    close()
    try { options.onEvent(event) } catch {
      report(callbackFailure, { code: 'public_projection_revoked_callback_failed' })
    }
  }
  const revoke = (): void => emitTerminal({
    schemaVersion: 1,
    kind: 'public_projection_revoked',
    threadId: options.threadId,
    historyAuthority: 'case_boundary_only_v1',
    code: 'case_public_authority_unavailable',
    action: 'purge_case_projection',
    terminal: true
  }, 'SSE authority-loss callback failed')

  const receive = async (body: ReadableStream<Uint8Array>): Promise<void> => {
    const current = body.getReader()
    reader = current
    const utf8 = new TextDecoder()
    let pending = ''
    try {
      for (;;) {
        const chunk = await current.read()
        if (stopped || chunk.done) return
        pending += utf8.decode(chunk.value, { stream: true })
        for (;;) {
          if (stopped) return
          const frame = takePublicRuntimeSseBlock(pending)
          if (frame === null) break
          pending = frame.rest
          const projected = projectPublicRuntimeSseBlock(frame.block, options.threadId, filter)
          if (projected === null) continue
          switch (projected.status) {
            case 'emit':
              options.onEvent(projected.event as RuntimeSseEvent)
              cursor = Math.max(cursor, projected.seq)
              break
            case 'revoke':
              emitTerminal(projected.event, 'SSE authority-revocation callback failed')
              return
            default:
              revoke()
              report('SSE event rejected', { code: 'sse_event_rejected', reasonCode: projected.reason })
              return
          }
        }
      }
    } finally {
      if (reader === current) reader = undefined
      void current.cancel().catch(() => undefined)
      current.releaseLock()
    }
  }
  const pause = (milliseconds: number): Promise<void> => new Promise((resolve) => {
    if (stopped) { resolve(); return }
    wake = resolve
    timer = setTimeout(() => {
      timer = undefined
      wake = undefined
      resolve()
    }, milliseconds)
  })
  const run = async (): Promise<void> => {
    let retry = 750
    while (!stopped) {
      try {
        endpoint.searchParams.set('since_seq', String(cursor))
        const response = await fetch(endpoint, {
          signal: abort.signal,
          headers: { ...options.headers, Accept: 'text/event-stream' }
        })
        if (stopped) { void response.body?.cancel().catch(() => undefined); return }
        if (response.ok && response.body) {
          retry = 750
          await receive(response.body)
        } else {
          void response.body?.cancel().catch(() => undefined)
          const code = response.status
          if (code >= 400 && code < 500 && code !== 408 && code !== 429) {
            revoke()
            report('SSE connection refused', { statusCode: code })
            return
          }
        }
      } catch {
        if (stopped) return
        report('SSE stream error', { code: 'stream_error' })
      }
      await pause(retry)
      retry = Math.min(5000, retry * 2)
    }
  }

  options.signal.addEventListener('abort', close, { once: true })
  if (options.signal.aborted) close()
  else void run()
  return { close }
}
