import { randomBytes } from 'node:crypto'
import { ipcMain, type BrowserWindow, type WebContents } from 'electron'
import { browserScopeSchema, browserSelectionPath, browserSelectionRequestSchema, type BrowserScope } from '../../../packages/runtime/src/contracts/browser-selection'
import { isAllowedDevPreviewUrl } from '../../shared/dev-preview-url'
import { captureNativeRendererDocument } from '../ipc/native-renderer-document'
import { browserSelectionScript } from './selection-script'

const worldId = 1004
const token = () => randomBytes(24).toString('hex')
const guests = new Map<number, { host: WebContents; guest: WebContents; hostLease: ReturnType<typeof captureNativeRendererDocument> }>()

/** Called only by Electron's did-attach-webview, never by renderer identity claims. */
export function registerBrowserGuest(host: WebContents, guest: WebContents): void {
  if (guests.has(guest.id) || guests.size >= 16 || guest.getType() !== 'webview') return
  const remove = () => { if (guests.get(guest.id) === entry) { guests.delete(guest.id); entry.hostLease.release() } }
  const frame = host.mainFrame
  const entry = { host, guest, hostLease: captureNativeRendererDocument(host, () => !host.isDestroyed() && host.mainFrame === frame, remove) }
  guests.set(guest.id, entry)
  guest.once('destroyed', remove)
}

type Transport = (path: string, body: string, signal?: AbortSignal) => Promise<{ ok: boolean; body: string }>
export function registerBrowserSelectionIpc(getMainWindow: () => BrowserWindow | null, transport: Transport): void {
  type Capture = { host: WebContents; scope: BrowserScope; current: () => boolean; check: () => Promise<boolean>; release: () => void; abort: AbortController }
  const captures = new Map<string, Capture>()
  let capturing = false
  const request = async (body: unknown, signal?: AbortSignal): Promise<Record<string, unknown>> => {
    const response = await transport(browserSelectionPath, JSON.stringify(body), signal)
    const value = JSON.parse(response.body)
    if (!response.ok || value?.ok !== true) throw Error('selection-unavailable')
    return value
  }
  const retire = (capture: Capture) => {
    if (captures.get(capture.scope.scopeId) !== capture) return
    captures.delete(capture.scope.scopeId); capture.abort.abort(); capture.release()
    void request({ action: 'revoke', scope: capture.scope }).catch(() => undefined)
  }
  const serve = async (capture: Capture) => {
    const expires = Date.now() + 5 * 60_000
    try {
      while (capture.current() && Date.now() < expires && captures.get(capture.scope.scopeId) === capture) {
        const answer = await request({ action: 'next', scope: capture.scope }, capture.abort.signal)
        if (answer.nonce === '') continue
        if (typeof answer.nonce !== 'string' || !/^[a-f0-9]{48}$/.test(answer.nonce)) break
        const current = capture.current() && await capture.check() && capture.current()
        await request({ action: 'answer', scope: capture.scope, nonce: answer.nonce, current }, capture.abort.signal)
        if (!current) break
      }
    } catch { /* Core expiry, shutdown and transport failures revoke this handle. */ }
    finally { retire(capture) }
  }
  ipcMain.handle('browser:selection', async (event, payload: unknown) => {
    const main = getMainWindow(), parsed = browserSelectionRequestSchema.safeParse(payload)
    if (!parsed.success || !main || main.isDestroyed() || event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame) return { ok: false }
    const input = parsed.data
    if (input.action !== 'capture') {
      const capture = captures.get(input.scope.scopeId)
      if (!capture || capture.host !== event.sender || JSON.stringify(capture.scope) !== JSON.stringify(input.scope) || !capture.current()) return { ok: false }
      if (input.action === 'revoke') { retire(capture); return { ok: true } }
      try {
        await request({ action: 'validate', scope: capture.scope })
        return { ok: capture.current() && captures.get(capture.scope.scopeId) === capture }
      } catch { retire(capture); return { ok: false } }
    }
    const entry = guests.get(input.guestId)
    if (!entry || entry.host !== event.sender || !entry.hostLease.isCurrent() || entry.guest.isDestroyed() ||
      entry.guest.isLoadingMainFrame() || !isAllowedDevPreviewUrl(entry.guest.getURL()) || capturing) return { ok: false }
    capturing = true
    const guest = entry.guest, frame = guest.mainFrame, selectionId = token(), documentId = token()
    let capture: Capture | undefined
    const lease = captureNativeRendererDocument(guest, () => guests.get(input.guestId) === entry && entry.hostLease.isCurrent() &&
      getMainWindow() === main && !main.isDestroyed() && !guest.isDestroyed() && guest.mainFrame === frame,
      () => { if (capture) retire(capture) })
    const current = () => lease.isCurrent() && entry.hostLease.isCurrent()
    const execute = (action: 'capture' | 'check' | 'release') => guest.executeJavaScriptInIsolatedWorld(worldId, [{ code: browserSelectionScript(action, selectionId) }])
    try {
      if (captures.size >= 8) retire(captures.values().next().value!)
      const text: unknown = await execute('capture')
      if (!current() || typeof text !== 'string' || !text.trim() || text.length > 4096 || Buffer.byteLength(text) > 16384) throw Error('selection-unavailable')
      const result = await request({ action: 'capture', capture: { threadId: input.threadId, documentId, selectionId, text } })
      const scope = browserScopeSchema.parse(result.scope)
      if (scope.documentId !== documentId || scope.selectionId !== selectionId || scope.threadId !== input.threadId) throw Error('selection-unavailable')
      capture = { host: event.sender, scope, current, check: async () => current() && await execute('check') === true && current(),
        release: () => { lease.release(); if (!guest.isDestroyed() && guest.mainFrame === frame) void execute('release').catch(() => undefined) }, abort: new AbortController() }
      captures.set(scope.scopeId, capture)
      if (!current() || !await capture.check()) { retire(capture); return { ok: false } }
      void serve(capture)
      return { ok: true, scope }
    } catch {
      if (capture) retire(capture)
      else { lease.release(); if (!guest.isDestroyed() && guest.mainFrame === frame) void execute('release').catch(() => undefined) }
      return { ok: false }
    } finally { capturing = false }
  })
}
