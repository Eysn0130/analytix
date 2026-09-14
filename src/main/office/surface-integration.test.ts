import { expect, test, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { isOfficeRequest } from './office-protocol'
const envelope = (command: string, extra = {}) => ({ command, channel: 'channel', documentId: 'document', version: 'version', operationId: command, ...extra })
function worker(readOnly = true) {
  const messages: any[] = [], listeners: any = {}, close = vi.fn(), remove = vi.fn()
  const controller = {
    getFrame: () => ({ getContainerWindow: () => ({}), LayoutManager: { setVisible() {}, isVisible: () => false } }),
    getSelection: () => ({ getString: () => '' }), addSelectionChangeListener() {}, removeSelectionChangeListener: remove
  }
  const load = vi.fn()
  const model = { isReadonly: vi.fn(() => readOnly), isModified: vi.fn(() => false), getCurrentController: () => controller, setModified: vi.fn(), close,
    addModifyListener: (l: any) => { listeners.modify = l }, removeModifyListener: remove, storeToURL() {} }
  const port: any = { postMessage: (m: any) => messages.push(m) }
  const css = { frame: { Desktop: { create: () => ({ loadComponentFromURL: (...args: unknown[]) => { load(...args); return model } }) } },
    beans: { PropertyValue: function(this: any, p: any) { Object.assign(this, p) } }, util: { XModifyListener: {} }, view: { XSelectionChangeListener: {} } }
  const zeta = { uno: { com: { sun: { star: css } } }, getUnoComponentContext() {}, mainPort: port, unoObject: (_: any, o: any) => o }
  vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js', import.meta.url), 'utf8'), { Module: { zetajs: { then: (fn: any) => fn(zeta) } } })
  const send = (command: string, extra = {}) => { port.onmessage({ data: envelope(command, extra) }); return messages.at(-1) }
  send('bind'); send('open', { kind: 'docx' })
  return { send, close, model, remove, load, change: () => listeners.modify.modified() }
}
test('conditional close accepts clean exact sequence and performs one native close', () => {
  const w = worker(); expect(w.send('close', { expectedChangeSequence: 0, discard: false }).ok).toBe(true); expect(w.close).toHaveBeenCalledOnce()
})
test('dirty or stale sequence close is rejected without removing listeners or closing model', () => {
  const w = worker(); w.model.isModified.mockReturnValue(true); w.change()
  for (const sequence of [0, 1]) expect(w.send('close', { expectedChangeSequence: sequence, discard: false })).toMatchObject({ ok: false, error: 'unsaved-changes' })
  expect(w.close).not.toHaveBeenCalled(); expect(w.remove).not.toHaveBeenCalled()
  expect(w.send('captureSelection').state).toMatchObject({ changeSequence: 1, acknowledgedSequence: 0, dirty: true })
  expect(w.send('close', { expectedChangeSequence: 1, discard: true }).ok).toBe(true); expect(w.close).toHaveBeenCalledOnce()
})
test('preview opens native files readonly and denies every mutation or export command', () => {
  const w = worker()
  const properties = w.load.mock.calls[0][3]
  expect(properties).toEqual(expect.arrayContaining([
    expect.objectContaining({ Name: 'ReadOnly', Value: true }),
    expect.objectContaining({ Name: 'LockEditDoc', Value: true }),
    expect.objectContaining({ Name: 'LockSave', Value: true }),
    expect.objectContaining({ Name: 'LockExport', Value: true })
  ]))
  for (const command of ['export', 'ack', 'bold', 'undo', 'redo', 'local-ui']) {
    expect(w.send(command)).toMatchObject({ ok: false, error: 'unsupported-command' })
  }
  expect(w.send('captureSelection').state).toMatchObject({ changeSequence: 0, dirty: false })
})
test('native engine that does not honor readonly is closed before publishing a document', () => {
  const w = worker(false)
  expect(w.close).toHaveBeenCalledOnce()
  expect(w.send('captureSelection')).toMatchObject({ ok: false, error: 'stale-document-version' })
})
test('malformed close never reaches native close even with discard set', () => {
  const w = worker()
  for (const value of [{}, { expectedChangeSequence: -1, discard: true }, { expectedChangeSequence: Infinity, discard: true }, { expectedChangeSequence: 0, discard: 'yes' }]) {
    expect(isOfficeRequest(envelope('close', value))).toBe(false)
    expect(w.send('close', value).error).toBe('invalid-request')
  }
  expect(w.close).not.toHaveBeenCalled()
  expect(isOfficeRequest(envelope('close', { expectedChangeSequence: 0, discard: false }))).toBe(true)
})
test('Main facade starts engine and sends typed correlated replies without exposing a DOM port', async () => {
  let handler: any
  const bridge = { connect: (h: any) => { handler = h }, send: vi.fn() }
  const nodes: any = Object.fromEntries(['qtcanvas','status','selection','preview','footer','kind','viewControls'].map(k => [k, { addEventListener() {}, setAttribute() {}, dataset: {}, focus() {}, querySelectorAll: () => [] }]))
  const context: any = { document: { getElementById: (id: string) => nodes[id], querySelectorAll: () => [] }, analytixOfficeSurface: bridge, addEventListener: vi.fn(), window: { addEventListener() {} }, crypto: { randomUUID: () => 'operation' } }
  vm.createContext(context)
  const source = readFileSync(new URL('./surface/office-surface.js', import.meta.url), 'utf8').replaceAll('export ', '')
  vm.runInContext(source + '\nglobalThis.Engine = OfficeEngineSurface;', context)
  context.Engine.prototype.start = vi.fn(async function(this: any, channel: string) { this.channel = channel })
  context.Engine.prototype.request = vi.fn(async (r: any) => ({ type: 'result', ...r, ok: true }))
  await handler({ type: 'bind', channel: 'channel' })
  expect(bridge.send).toHaveBeenCalledWith({ type: 'ready', channel: 'channel', protocolVersion: 1 })
  // Create the request in the page realm to match the production contextBridge clone.
  context.deliver = handler
  await vm.runInContext("deliver({command:'close',channel:'channel',operationId:'close',documentId:'document',version:'version',expectedChangeSequence:0,discard:false})", context)
  expect(context.Engine.prototype.request).toHaveBeenCalledOnce()
  context.Engine.prototype.request.mockRejectedValueOnce(new Error('/private/document content'))
  await vm.runInContext("deliver({command:'bold',channel:'channel',operationId:'bold',documentId:'document',version:'version'})", context)
  expect(bridge.send.mock.calls.at(-1)?.[0]).toMatchObject({ type: 'result', ok: false, error: 'engine-operation-failed' })
  expect(JSON.stringify(bridge.send.mock.calls)).not.toContain('/private/document')
})

test('preview input guard blocks mutation and preserves navigation, selection and copy', async () => {
  const listeners = new Map<string, (event: any) => void>()
  const context: any = { document: { getElementById: () => null }, window: { addEventListener: (type: string, handler: any) => listeners.set(type, handler) } }
  vm.createContext(context)
  const source = readFileSync(new URL('./surface/office-surface.js', import.meta.url), 'utf8').replaceAll('export ', '')
  vm.runInContext(source + '\nglobalThis.engine = new OfficeEngineSurface({}, () => {});', context)
  context.engine.channel = 'channel'
  context.engine.worker = vi.fn()
  for (const command of ['export', 'ack', 'bold', 'undo', 'redo', 'local-ui']) {
    await expect(context.engine.request(envelope(command))).rejects.toThrow('unsupported-command')
  }
  expect(context.engine.worker).not.toHaveBeenCalled()
  for (const key of ['a', '中', 'Backspace', 'Delete', 'Enter', 'F2']) {
    const e = { key, preventDefault: vi.fn(), stopImmediatePropagation: vi.fn() }
    listeners.get('keydown')!(e)
    expect(e.preventDefault).toHaveBeenCalledOnce()
    expect(e.stopImmediatePropagation).toHaveBeenCalledOnce()
  }
  for (const key of ['s', 'b', 'z', 'v', 'x']) {
    const e = { key, ctrlKey: true, preventDefault: vi.fn(), stopImmediatePropagation: vi.fn() }
    listeners.get('keydown')!(e)
    expect(e.preventDefault).toHaveBeenCalledOnce()
  }
  for (const type of ['beforeinput', 'paste', 'drop', 'cut']) {
    const e = { preventDefault: vi.fn(), stopImmediatePropagation: vi.fn() }
    listeners.get(type)!(e)
    expect(e.stopImmediatePropagation).toHaveBeenCalledOnce()
  }
  for (const input of [{ key: 'ArrowDown' }, { key: 'PageDown' }, { key: 'c', metaKey: true }]) {
    const e = { ...input, preventDefault: vi.fn(), stopImmediatePropagation: vi.fn() }
    listeners.get('keydown')!(e)
    expect(e.preventDefault).not.toHaveBeenCalled()
  }
  expect(listeners.has('wheel')).toBe(false)
})


test('native view notifications preserve the clean sequence without clearing modified state', () => {
  const w = worker()
  w.change()
  expect(w.send('captureSelection').state).toMatchObject({ changeSequence: 0, dirty: false })
  expect(w.model.setModified).not.toHaveBeenCalled()
  expect(w.send('close', { expectedChangeSequence: 0, discard: false }).ok).toBe(true)
  expect(w.model.setModified).not.toHaveBeenCalled()
})

test.each(['readonly-lost', 'readonly-failed', 'modified-failed'])('unknown native view state stays fail-closed: %s', (reason) => {
  const w = worker()
  if (reason === 'readonly-lost') w.model.isReadonly.mockReturnValue(false)
  else if (reason === 'readonly-failed') w.model.isReadonly.mockImplementation(() => { throw Error('unknown') })
  else w.model.isModified.mockImplementation(() => { throw Error('unknown') })
  w.change()
  expect(w.send('captureSelection').state).toMatchObject({ changeSequence: 1, dirty: true })
  expect(w.model.setModified).not.toHaveBeenCalled()
})
