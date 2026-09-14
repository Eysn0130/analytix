import { afterEach, expect, test, vi } from 'vitest'
const mocked = vi.hoisted(() => ({ ipc: undefined as any, facade: undefined as any }))
vi.mock('electron', async () => {
  const { EventEmitter } = await import('node:events')
  mocked.ipc = new EventEmitter()
  return { ipcRenderer: mocked.ipc, contextBridge: { exposeInMainWorld(name: string, value: any) { expect(name).toBe('analytixOfficeSurface'); mocked.facade = value } } }
})
afterEach(() => { vi.unstubAllGlobals() })
test('only a matching Main IPC transfers the port; page facade is typed and single-connect', async () => {
  const url = 'http://127.0.0.1:43219/11111111-1111-1111-1111-111111111111/office-surface.html'
  vi.stubGlobal('location', new URL(url)); vi.stubGlobal('addEventListener', vi.fn())
  await import('./office-preload')
  const port = { postMessage: vi.fn(), start: vi.fn(), close: vi.fn(), onmessage: undefined as any }, events: any[] = []
  mocked.facade.connect((r: any) => events.push(r))
  mocked.ipc.emit('unrelated', { ports: [port] }, { channel: 'channel', entryURL: url })
  mocked.ipc.emit('analytix-office-surface-bind-v1', { ports: [port] }, { channel: 'channel', entryURL: 'https://outside.invalid' })
  expect(events).toEqual([])
  mocked.ipc.emit('analytix-office-surface-bind-v1', { ports: [port] }, { channel: 'channel', entryURL: url })
  expect(events).toEqual([{ type: 'bind', channel: 'channel' }]); expect(port.start).toHaveBeenCalledOnce()
  mocked.facade.send({ type: 'ready', channel: 'channel', protocolVersion: 1 })
  expect(port.postMessage).toHaveBeenCalledOnce()
  expect(() => mocked.facade.connect(() => {})).toThrow('office-transport-unavailable')
  expect(() => mocked.facade.send({ type: 'request', channel: 'channel', command: 'eval', script: 'unsafe' })).toThrow('office-invalid-message')
  expect(() => mocked.facade.send({ type: 'fatal', channel: 'channel', error: 'raw private details' })).toThrow('office-invalid-message')
  port.onmessage({ data: { channel: 'channel', command: 'open', operationId: 'open', documentId: 'doc', version: 'v1', kind: 'docx', bytes: new Uint8Array([1]) } })
  expect(events.at(-1).command).toBe('open')
  const count = events.length
  for (const command of ['eval', 'export', 'ack', 'bold', 'undo', 'redo', 'local-ui']) {
    port.onmessage({ data: { channel: 'channel', command, operationId: command, documentId: 'doc', version: 'v1', ...(command === 'ack' ? { status: 'committed', persistedVersion: 'v2' } : {}) } })
    expect(events.length).toBe(count)
  }
  expect(() => mocked.facade.send({ type: 'save-requested', channel: 'channel', operationId: 'save', documentId: 'doc', version: 'v1' })).toThrow('office-invalid-message')
  expect(() => mocked.facade.send({ type: 'result', command: 'export', channel: 'channel', operationId: 'export', documentId: 'doc', version: 'v1', ok: false, error: 'unsupported-command' })).toThrow('office-invalid-message')
  port.onmessage({ data: { channel: 'channel', command: 'captureSelection', operationId: 'capture', documentId: 'doc', version: 'v1' } })
  expect(events.at(-1).command).toBe('captureSelection')
  port.onmessage({ data: { channel: 'channel', command: 'close', operationId: 'close', documentId: 'doc', version: 'v1', expectedChangeSequence: 0, discard: false } })
  expect(events.at(-1).command).toBe('close')
  expect(Object.keys(mocked.facade).sort()).toEqual(['connect', 'send'])
})
