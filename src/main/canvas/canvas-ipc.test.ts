import { beforeEach, expect, it, vi } from 'vitest'
import { EventEmitter } from 'node:events'
import type { PluginPackageHostRequest, PluginPackageHostResponse, PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'

const state = vi.hoisted(() => ({ handlers: new Map<string, (...args: any[]) => any>(), picker: vi.fn(), owner: vi.fn() }))
vi.mock('electron', () => ({ ipcMain: { handle: (name: string, handler: (...args: any[]) => any) => state.handlers.set(name, handler) },
  dialog: { showOpenDialog: state.picker }, BrowserWindow: { fromWebContents: state.owner } }))
import { registerCanvasIpc } from './canvas-ipc'
beforeEach(() => { vi.clearAllMocks(); state.handlers.clear() })

it.each(['navigation', 'render-process-gone', 'destroyed', 'iframe'])('binds Canvas document bytes to the initiating document across %s', async reason => {
  const frame = {}, sender = Object.assign(new EventEmitter(), { mainFrame: frame, isDestroyed: () => false })
  const hash = 'b'.repeat(64)
  const pkg: PluginPackageView = { packageId: 'analytix-canvas', packageVersion: '0.1.0', displayName: 'Canvas', origin: 'development-source', publishable: false, materialized: true,
    generationId: hash, activationState: 'recorded', desiredState: 'enabled', activationRevision: 1, activationId: 'c'.repeat(64), available: true,
    operations: ['open-object', 'read-object', 'close-object'] }
  const document = { sessionId: 'a'.repeat(48), objectId: hash, threadId: 'thread-main', kind: 'canvas', path: '/workspace/diagram.canvas', revision: hash, content: 'e30=' }
  const packageHost = vi.fn(async (request: PluginPackageHostRequest): Promise<PluginPackageHostResponse> => {
    if (request.action === 'list') return { ok: true, packages: [pkg] }
    if (request.action === 'invoke' && request.operation === 'open-object') {
      if (reason === 'navigation' || reason === 'iframe') sender.emit('did-start-navigation', {}, 'analytix://app', false, reason !== 'iframe')
      else sender.emit(reason)
      return { ok: true, output: { ok: true, document } }
    }
    return { ok: true, output: { ok: true, closed: true } }
  })
  registerCanvasIpc(() => true, packageHost)
  const result = await state.handlers.get('canvas:request')!({ sender, senderFrame: frame },
    { operation: 'open-object', threadId: 'thread-main', kind: 'canvas', object: { workspace: '/workspace', path: 'diagram.canvas' } })
  if (reason === 'iframe') {
    expect(result).toEqual({ ok: true, document })
    sender.emit('destroyed')
    return
  }
  expect(result).toEqual({ ok: false, code: 'unavailable' })
  expect(packageHost.mock.calls.some(([request]) => request.action === 'invoke' && request.operation === 'close-object')).toBe(true)
  expect(sender.eventNames()).toEqual([])
})

it('does not return a picked path after same-frame navigation and releases picker listeners', async () => {
  const frame = {}, sender = Object.assign(new EventEmitter(), { mainFrame: frame, isDestroyed: () => false })
  state.owner.mockReturnValue({ isDestroyed: () => false, webContents: sender })
  const packageHost = vi.fn(async () => ({ ok: true, packages: [{ packageId: 'analytix-canvas', available: true, desiredState: 'enabled' }] }) as PluginPackageHostResponse)
  state.picker.mockImplementationOnce(async () => {
    sender.emit('did-start-navigation', {}, 'analytix://app', false, true)
    return { canceled: false, filePaths: ['/workspace/diagram.canvas'] }
  })
  registerCanvasIpc(() => true, packageHost)
  expect(await state.handlers.get('canvas:pick-file')!({ sender, senderFrame: frame }, { workspace: '/workspace' })).toEqual({ ok: false })
  expect(sender.eventNames()).toEqual([])
})

it('finishes old Canvas cleanup before a replacement document reopens the same Core session', async () => {
  const frame = {}, sender = Object.assign(new EventEmitter(), { mainFrame: frame, isDestroyed: () => false })
  const hash = 'b'.repeat(64)
  const pkg: PluginPackageView = { packageId: 'analytix-canvas', packageVersion: '0.1.0', displayName: 'Canvas', origin: 'development-source', publishable: false, materialized: true,
    generationId: hash, activationState: 'recorded', desiredState: 'enabled', activationRevision: 1, activationId: 'c'.repeat(64), available: true,
    operations: ['open-object', 'read-object', 'close-object'] }
  const document = { sessionId: 'a'.repeat(48), objectId: hash, threadId: 'thread-main', kind: 'canvas', path: '/workspace/diagram.canvas', revision: hash, content: 'e30=' }
  let finishOld!: () => void, finishClose!: () => void, opens = 0
  const order: string[] = []
  const packageHost = vi.fn(async (request: PluginPackageHostRequest): Promise<PluginPackageHostResponse> => {
    if (request.action === 'list') return { ok: true, packages: [pkg] }
    if (request.action === 'invoke' && request.operation === 'open-object') {
      opens++; order.push(`open-${opens}`)
      if (opens === 1) await new Promise<void>(resolve => { finishOld = resolve })
      return { ok: true, output: { ok: true, document } }
    }
    order.push('close-start')
    await new Promise<void>(resolve => { finishClose = resolve })
    order.push('close-end')
    return { ok: true, output: { ok: true, closed: true } }
  })
  registerCanvasIpc(() => true, packageHost)
  const event = { sender, senderFrame: frame }
  const open = { operation: 'open-object', threadId: 'thread-main', kind: 'canvas', object: { workspace: '/workspace', path: 'diagram.canvas' } }
  const first = state.handlers.get('canvas:request')!(event, open)
  await vi.waitFor(() => expect(opens).toBe(1))
  sender.emit('did-start-navigation', {}, 'analytix://app', false, true)
  const second = state.handlers.get('canvas:request')!(event, open)
  finishOld()
  await vi.waitFor(() => expect(finishClose).toBeTypeOf('function'))
  expect(opens).toBe(1)
  finishClose()
  expect(await first).toEqual({ ok: false, code: 'unavailable' })
  expect(await second).toEqual({ ok: true, document })
  expect(order).toEqual(['open-1', 'close-start', 'close-end', 'open-2'])
})
