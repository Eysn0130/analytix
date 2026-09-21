import { EventEmitter } from 'node:events'
import { afterEach, expect, test, vi } from 'vitest'
const state = vi.hoisted(() => ({ handlers: new Map<string, (...args: any[]) => Promise<any>>() }))
vi.mock('electron', () => ({ ipcMain: { handle: (name: string, fn: any) => state.handlers.set(name, fn) } }))
import { registerBrowserGuest, registerBrowserSelectionIpc } from './browser-selection'
const cleanups: Array<() => void> = []
afterEach(() => { for (const dispose of cleanups.splice(0)) dispose() })
let nextId = 0
function fixture() {
  const host = Object.assign(new EventEmitter(), { mainFrame: {}, isDestroyed: () => false })
  let loading = false, valid = true, destroyed = false
  const guest = Object.assign(new EventEmitter(), { id: ++nextId, mainFrame: {}, getType: () => 'webview',
    getURL: () => 'http://127.0.0.1:1234/', isDestroyed: () => destroyed, isLoadingMainFrame: () => loading,
    executeJavaScriptInIsolatedWorld: vi.fn(async (_world: number, scripts: {code: string}[]) => scripts[0].code.endsWith(`"capture",`) ? false : scripts[0].code.includes(')("capture",') ? 'selected text' : valid) })
  const main = { webContents: host, isDestroyed: () => false }
  let deliver: ((v: any) => void) | undefined, validated: ((v: any) => void) | undefined
  const response = (body: object) => ({ ok: true, body: JSON.stringify(body) })
  let scope: any, captureId = 0
  const transport = vi.fn(async (_path: string, body: string, signal?: AbortSignal) => {
    const r = JSON.parse(body)
    if (r.action === 'capture') { scope = { scopeId: (++captureId).toString(16).padStart(48, '0'), workspace: '/workspace', threadId: r.capture.threadId, documentId: r.capture.documentId, selectionId: r.capture.selectionId }; return response({ ok: true, scope }) }
    if (r.action === 'next') return await new Promise<any>((resolve, reject) => { deliver = resolve; signal?.addEventListener('abort', () => reject(Error('aborted')), { once: true }) })
    if (r.action === 'validate') return await new Promise<any>(resolve => { validated = resolve; deliver?.(response({ ok: true, nonce: 'd'.repeat(48) })) })
    if (r.action === 'answer') { validated?.(response({ ok: r.current })); return response({ ok: true }) }
    return response({ ok: true })
  })
  registerBrowserGuest(host as any, guest as any)
  registerBrowserSelectionIpc(() => main as any, transport)
  const event = { sender: host, senderFrame: host.mainFrame }
  const invoke = (payload: any, sender: any = event) => state.handlers.get('browser:selection')!(sender, payload)
  cleanups.push(() => { destroyed = true; guest.emit('destroyed'); host.emit('destroyed') })
  return { host, guest, main, event, invoke, transport, capture: () => invoke({ action: 'capture', guestId: guest.id, threadId: 'thread-1' }),
    invalidate: () => { valid = false }, load: () => { loading = true }, scope: () => scope }
}
test('only registered owner main frame can capture; no renderer text enters Core', async () => {
  const h = fixture()
  expect(await h.invoke({ action: 'capture', guestId: h.guest.id, threadId: 'thread-1' }, { sender: h.host, senderFrame: {} })).toEqual({ ok: false })
  expect(await h.invoke({ action: 'capture', guestId: h.guest.id + 1, threadId: 'thread-1' })).toEqual({ ok: false })
  expect(await h.invoke({ action: 'capture', guestId: h.guest.id, threadId: 'thread-1', text: 'forged' })).toEqual({ ok: false })
  expect(h.transport).not.toHaveBeenCalled()
  expect(await h.capture()).toMatchObject({ ok: true, scope: { threadId: 'thread-1' } })
  expect(h.guest.executeJavaScriptInIsolatedWorld.mock.calls[0][0]).toBe(1004)
  expect(JSON.parse(h.transport.mock.calls[0][1]).capture.text).toBe('selected text')
  expect(await h.invoke({ action: 'validate', scope: h.scope() })).toEqual({ ok: true })
  h.invalidate()
  expect(await h.invoke({ action: 'validate', scope: h.scope() })).toEqual({ ok: false })
})
test.each(['same-url', 'same-document', 'provisional-cancel', 'crash', 'host-navigation', 'destroy'])('irreversibly rejects %s and late replies', async mode => {
  const h = fixture(); await h.capture()
  if (mode === 'crash') h.guest.emit('render-process-gone', {}, {})
  else if (mode === 'destroy') h.guest.emit('destroyed')
  else (mode === 'host-navigation' ? h.host : h.guest).emit('did-start-navigation', { isMainFrame: true, isSameDocument: mode === 'same-document' }, 'http://127.0.0.1:1234/', mode === 'same-document', true, 1, 1)
  if (mode === 'provisional-cancel') h.guest.emit('did-fail-provisional-load', {}, -3, 'aborted', '', true)
  expect(await h.invoke({ action: 'validate', scope: h.scope() })).toEqual({ ok: false })
})
test('iframe navigation does not revoke a main-frame capture; loading main frame cannot capture', async () => {
  const h = fixture(); await h.capture(); h.guest.emit('did-start-navigation', { isMainFrame: false }, 'http://127.0.0.1:1234/frame', false, false, 1, 2)
  expect(await h.invoke({ action: 'validate', scope: h.scope() })).toEqual({ ok: true })
  h.load(); expect(await h.capture()).toEqual({ ok: false })
})
test('navigation during isolated extraction prevents Core capture', async () => {
  const h = fixture(); let finish!: (v: unknown) => void
  h.guest.executeJavaScriptInIsolatedWorld.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }) as any)
  const pending = h.capture(); h.guest.emit('did-start-navigation', { isMainFrame: true }, '', false, true, 1, 1); finish('old text')
  expect(await pending).toEqual({ ok: false }); expect(h.transport).not.toHaveBeenCalled()
})
test('concurrent captures cannot overtake the bounded owner', async () => {
  const h = fixture(); let finish!: (v: unknown) => void
  h.guest.executeJavaScriptInIsolatedWorld.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }) as any)
  const pending = h.capture()
  expect(await h.capture()).toEqual({ ok: false })
  finish('selected text')
  expect(await pending).toMatchObject({ ok: true })
})

test('ninth capture retires the oldest scope without exceeding the Main bound', async () => {
  const h = fixture(); const first = await h.capture()
  for (let i = 0; i < 8; i++) expect(await h.capture()).toMatchObject({ ok: true })
  expect(await h.invoke({ action: 'validate', scope: first.scope })).toEqual({ ok: false })
  expect(h.transport.mock.calls.some(([, body]) => { const value = JSON.parse(body); return value.action === 'revoke' && value.scope.scopeId === first.scope.scopeId })).toBe(true)
})
test('navigation during Core capture retires its late scope', async () => {
  const h = fixture(); let finish!: (v: any) => void
  h.transport.mockImplementationOnce(async (_path, body) => await new Promise(resolve => {
    const { capture } = JSON.parse(body)
    finish = () => resolve({ ok: true, body: JSON.stringify({ ok: true, scope: {
      scopeId: 'c'.repeat(48), workspace: '/workspace', threadId: capture.threadId,
      documentId: capture.documentId, selectionId: capture.selectionId
    } }) })
  }))
  const pending = h.capture(); await vi.waitFor(() => expect(finish).toBeTypeOf('function'))
  h.guest.emit('did-start-navigation', { isMainFrame: true }, '', false, true, 1, 1)
  finish(undefined); expect(await pending).toEqual({ ok: false })
  expect(h.transport.mock.calls.some(([, body]) => JSON.parse(body).action === 'revoke')).toBe(true)
})
