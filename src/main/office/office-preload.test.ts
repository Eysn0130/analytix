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
  for (const command of ['eval', 'ack', 'bold', 'format', 'undo', 'redo', 'local-ui']) {
    port.onmessage({ data: { channel: 'channel', command, operationId: command, documentId: 'doc', version: 'v1', ...(command === 'ack' ? { status: 'committed', persistedVersion: 'v2' } : {}) } })
    expect(events.length).toBe(count)
  }
  mocked.facade.send({ type: 'save-requested', channel: 'channel', operationId: 'save', documentId: 'doc', version: 'v1' })
  expect(port.postMessage).toHaveBeenLastCalledWith(expect.objectContaining({type:'save-requested'}))
  mocked.facade.send({ type: 'result', command: 'export', channel: 'channel', operationId: 'export', documentId: 'doc', version: 'v1', ok: false, error: 'unsupported-command' })
  for (const command of ['edit','export']) {
    port.onmessage({data:{channel:'channel',command,operationId:command,documentId:'doc',version:'v1'}})
    expect(events.at(-1).command).toBe(command)
  }
  const replace={channel:'channel',command:'replace',operationId:'replace',documentId:'doc',version:'v1',selectionToken:'selection',expectedChangeSequence:0,text:'AI text',valueType:'text'}
  port.onmessage({data:replace});expect(events.at(-1).command).toBe('replace')
  const cell={sheet:0,column:0,row:0,text:'1',formula:'1',value:1,valueType:'number',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}
  const before={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0,cells:[cell]}
  const typed={channel:'channel',command:'replaceCells',operationId:'typed_replace',documentId:'doc',version:'v1',selectionToken:'selection',expectedChangeSequence:0,workbook:{before,after:{...before,cells:[{...cell,value:2,text:'2',formula:'2'}]},results:['']}}
  port.onmessage({data:typed});expect(events.at(-1)).toEqual(typed)
  const typedCount=events.length;port.onmessage({data:{...typed,path:'/private'}});expect(events.length).toBe(typedCount)
  const acceptedCount=events.length
  for(const valueType of ['number','formula']) { port.onmessage({data:{...replace,valueType}});expect(events.length).toBe(acceptedCount) }
  port.onmessage({ data: { channel: 'channel', command: 'captureSelection', operationId: 'capture', documentId: 'doc', version: 'v1' } })
  expect(events.at(-1).command).toBe('captureSelection')
  port.onmessage({ data: { channel: 'channel', command: 'close', operationId: 'close', documentId: 'doc', version: 'v1', expectedChangeSequence: 0, discard: false } })
  expect(events.at(-1).command).toBe('close')
  expect(Object.keys(mocked.facade).sort()).toEqual(['connect', 'send'])
})
