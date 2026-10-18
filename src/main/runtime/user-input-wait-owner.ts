import { UserInputWaitSnapshotSchemaV1 } from '../../../packages/runtime/src/contracts/approvals'
import { USER_INPUT_LOCAL_DISPLAY_PATH_V1, USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1, userInputLocalDisplaySchemaV1, userInputLocalSelectorSchemaV1, type UserInputLocalSelectorV1 } from '../../../packages/runtime/src/contracts/user-input-local-display'
import type { UserInputWaitOwnerEventV1, UserInputWaitOwnerStateV1 } from '../../shared/analytix-api'

export type UserInputWaitTransportV1 = (selector: UserInputLocalSelectorV1, signal: AbortSignal) => Promise<{
  body: ReadableStream<Uint8Array>; isCurrent: () => boolean
}>
type UserInputWaitTransportOptions<Settings, Pin extends { runtimeUrl: string }> = {
  loadSettings: () => Promise<Settings>
  ensureRuntime: (settings: Settings) => Promise<Settings | void>
  captureAuthorityPin: () => Pin | null
  isCurrentAuthorityPin: (pin: Pin) => boolean
  getBaseUrl: (settings: Settings) => string
  authHeaders: (settings: Settings) => Headers
  privateDisplayRequest: (settings: Settings, path: string, body: string, signal: AbortSignal) => Promise<{ ok: boolean; status: number; body: string }>
}

// Stateless Main composition seam: the private preflight proves the full
// source identity; the held GET itself carries only the canonical Host ID.
export function createUserInputWaitTransportV1<Settings, Pin extends { runtimeUrl: string }>(options: UserInputWaitTransportOptions<Settings, Pin>): UserInputWaitTransportV1 {
  return async (payload, signal) => {
    try {
      const selector = userInputLocalSelectorSchemaV1.parse(payload)
      const initial = await options.loadSettings()
      const settings = await options.ensureRuntime(initial) ?? initial
      const pin = options.captureAuthorityPin()
      if (signal.aborted || !pin || !options.isCurrentAuthorityPin(pin)) throw new Error('Unavailable')
      const base = new URL(options.getBaseUrl(settings))
      if (base.origin !== pin.runtimeUrl || base.pathname !== '/' || base.search || base.hash || base.username || base.password ||
          base.protocol !== 'http:' || !['127.0.0.1', '[::1]', 'localhost'].includes(base.hostname)) throw new Error('Unavailable')
      const raw = await options.privateDisplayRequest(settings, USER_INPUT_LOCAL_DISPLAY_PATH_V1, JSON.stringify(selector), signal)
      if (!raw.ok || raw.status !== 200 || Buffer.byteLength(raw.body, 'utf8') > USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1) throw new Error('Unavailable')
      const display = userInputLocalDisplaySchemaV1.parse(JSON.parse(raw.body))
      if (display.readOnly || display.status !== 'pending' || !display.timedWait ||
          Object.entries(selector).some(([key, value]) => display[key as keyof typeof selector] !== value) ||
          !options.isCurrentAuthorityPin(pin) || signal.aborted) throw new Error('Unavailable')
      const headers = new Headers(options.authHeaders(settings))
      headers.set('Accept', 'text/event-stream')
      const response = await fetch(new URL(`/v1/user-inputs/${selector.requestId}/wait`, base), {
        method: 'GET', headers, signal, redirect: 'error', cache: 'no-store'
      })
      if (!response.ok || response.status !== 200 || !response.body ||
          !/^text\/event-stream(?:\s*;|$)/i.test(response.headers.get('content-type') ?? '') ||
          !options.isCurrentAuthorityPin(pin) || signal.aborted) {
        await response.body?.cancel()
        throw new Error('Unavailable')
      }
      return { body: response.body, isCurrent: () => options.isCurrentAuthorityPin(pin) }
    } catch { throw new Error('Question wait unavailable.') }
  }
}
type DocumentOwner = { isCurrent: () => boolean; release: () => void }
type Claim = { selector: UserInputLocalSelectorV1; controller: AbortController; owner: DocumentOwner;
  wait: UserInputWaitOwnerStateV1; runtimeCurrent?: () => boolean; check?: ReturnType<typeof setInterval>; firstFrameTimeout?: ReturnType<typeof setTimeout> }
const unavailable = (): UserInputWaitOwnerStateV1 => ({ state: 'unavailable' })

// One Main-owned held connection per document/request. Leaf removal, selection
// and window visibility do not release this business claim. No raw bytes or
// local countdown live here; the Host remains the settlement authority.
export class UserInputWaitOwnerV1 {
  private claims = new Map<string, Claim>()
  constructor(private transport: UserInputWaitTransportV1, private notify: (event: UserInputWaitOwnerEventV1) => void) {}
  private key(selector: UserInputLocalSelectorV1): string { return JSON.stringify(selector) }
  close(selector: UserInputLocalSelectorV1, notifyUnavailable = false): void {
    const key = this.key(selector), claim = this.claims.get(key)
    if (!claim) return
    this.claims.delete(key)
    if (notifyUnavailable && claim.owner.isCurrent()) {
      try { this.notify({ selector, wait: unavailable() }) } catch { /* Retired document. */ }
    }
    clearInterval(claim.check)
    clearTimeout(claim.firstFrameTimeout)
    claim.controller.abort()
    claim.owner.release()
  }
  invalidate(): void { for (const claim of [...this.claims.values()]) this.close(claim.selector, true) }
  open(payload: unknown, owner: DocumentOwner): UserInputWaitOwnerStateV1 {
    const parsed = userInputLocalSelectorSchemaV1.safeParse(payload)
    if (!parsed.success || !owner.isCurrent()) { owner.release(); return unavailable() }
    const selector = parsed.data, key = this.key(selector), previous = this.claims.get(key)
    if (previous && previous.owner.isCurrent() && (!previous.runtimeCurrent || previous.runtimeCurrent())) {
      owner.release(); return previous.wait
    }
    this.close(selector)
    if (this.claims.size >= 32) { owner.release(); return unavailable() }
    const claim: Claim = { selector, owner, controller: new AbortController(), wait: { state: 'connecting' } }
    this.claims.set(key, claim)
    const current = () => this.claims.get(key) === claim && !claim.controller.signal.aborted &&
      claim.owner.isCurrent() && (!claim.runtimeCurrent || claim.runtimeCurrent())
    // Delayed callbacks from a retired connection cannot close its successor.
    const retireCurrent = () => { if (this.claims.get(key) === claim) this.close(selector, true) }
    const publish = (wait: UserInputWaitOwnerStateV1) => {
      if (!current()) { retireCurrent(); return }
      claim.wait = wait
      try { this.notify({ selector, wait }) } catch { /* A destroyed document retires independently. */ }
    }
    claim.check = setInterval(() => { if (!current()) retireCurrent() }, 1000)
    const firstFrameTimeout = claim.firstFrameTimeout = setTimeout(() => { publish(unavailable()); claim.controller.abort() }, 30_000)
    void (async () => {
      let reader: ReadableStreamDefaultReader<Uint8Array> | undefined
      try {
        const stream = await this.transport(selector, claim.controller.signal)
        claim.runtimeCurrent = stream.isCurrent
        if (!current()) { retireCurrent(); return }
        reader = stream.body.getReader()
        const decoder = new TextDecoder('utf-8', { fatal: true })
        let pending = '', framed = false
        for (;;) {
          const chunk = await reader.read()
          if (!current()) { retireCurrent(); return }
          if (chunk.done) {
            // EOF proves only that this connection ended. It is not continued.
            pending += decoder.decode()
            if (!framed || pending.trim() !== '') throw new Error('Invalid wait frame')
            publish({ state: 'ended' }); return
          }
          pending = (pending + decoder.decode(chunk.value, { stream: true })).replace(/\r\n/g, '\n')
          if (pending.length > 4096) throw new Error('Invalid wait frame')
          const boundary = pending.indexOf('\n\n')
          if (boundary < 0) continue
          const frame = pending.slice(0, boundary)
          pending = pending.slice(boundary + 2)
          if (framed || pending.trim() !== '') throw new Error('Invalid wait frame')
          const lines = frame.split('\n')
          const data = lines.filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
          if (!data || lines.some(line => line !== '' && !line.startsWith('data:') && !line.startsWith(':'))) throw new Error('Invalid wait frame')
          const snapshot = UserInputWaitSnapshotSchemaV1.safeParse(JSON.parse(data))
          if (!snapshot.success) throw new Error('Invalid wait frame')
          framed = true
          clearTimeout(firstFrameTimeout)
          publish({ state: 'waiting', snapshot: snapshot.data, observedAt: Date.now() })
        }
      } catch { publish(unavailable()) }
      finally {
        clearTimeout(firstFrameTimeout)
        claim.controller.abort()
        try { await reader?.cancel() } catch { /* Transport already closed. */ }
        reader?.releaseLock()
        if (this.claims.get(key) === claim) {
          this.claims.delete(key); clearInterval(claim.check); claim.owner.release()
        }
      }
    })()
    return claim.wait
  }
}
