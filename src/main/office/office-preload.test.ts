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
  const typed={channel:'channel',command:'replaceCells',operationId:'typed_replace',documentId:'doc',version:'a'.repeat(64),selectionToken:'selection',expectedChangeSequence:0,workbook:{before,after:{...before,cells:[{...cell,value:2,text:'2',formula:'2'}]},results:['']},candidate:{changeId:'b'.repeat(64),saveOperationId:'save_workbook_01',baseRevision:'a'.repeat(64)}}
  port.onmessage({data:typed});expect(events.at(-1)).toEqual(typed)
  const typedCount=events.length;port.onmessage({data:{...typed,path:'/private'}});expect(events.length).toBe(typedCount)
  const acceptedCount=events.length
  for(const valueType of ['number','formula']) { port.onmessage({data:{...replace,valueType}});expect(events.length).toBe(acceptedCount) }
  port.onmessage({ data: { channel: 'channel', command: 'captureSelection', operationId: 'capture', documentId: 'doc', version: 'v1' } })
  expect(events.at(-1).command).toBe('captureSelection')
  port.onmessage({ data: { channel: 'channel', command: 'close', operationId: 'close', documentId: 'doc', version: 'v1', expectedChangeSequence: 0, discard: false } })
  expect(events.at(-1).command).toBe('close')
  for(const action of ['read','fit','zoom-in','zoom-out','previous-page','next-page']) {
    const request={channel:'channel',command:'local-view',operationId:action,documentId:'doc',version:'v1',action}
    port.onmessage({data:request}); expect(events.at(-1)).toEqual(request)
  }
  const preset={channel:'channel',command:'local-view',operationId:'preset',documentId:'doc',version:'v1',action:'set-zoom',zoom:25}
  port.onmessage({data:preset});expect(events.at(-1)).toEqual(preset)
  const controlCount=events.length
  for(const extra of [{zoom:19},{zoom:401},{zoom:25.5},{action:['fit'],zoom:undefined},{path:'/private'}]) {
    port.onmessage({data:{...preset,...extra}});expect(events.length).toBe(controlCount)
  }
  const viewEvent={type:'view',channel:'channel',operationId:'opened',documentId:'doc',version:'v1',view:{zoom:25,zoomType:3,fit:false,page:null,pages:null,readOnly:true,modified:false}}
  mocked.facade.send(viewEvent);expect(port.postMessage).toHaveBeenLastCalledWith(viewEvent)
  mocked.facade.send({...viewEvent,view:null});expect(port.postMessage).toHaveBeenLastCalledWith({...viewEvent,view:null})
  const writableEvent={...viewEvent,view:{...viewEvent.view,readOnly:false}}
  mocked.facade.send(writableEvent);expect(port.postMessage).toHaveBeenLastCalledWith(writableEvent)
  for(const readOnly of ['false',0,null,undefined]) {
    expect(()=>mocked.facade.send({...viewEvent,view:{...viewEvent.view,readOnly}})).toThrow('office-invalid-message')
  }
  expect(()=>mocked.facade.send({...viewEvent,view:{...viewEvent.view,extra:true}})).toThrow('office-invalid-message')
  expect(() => mocked.facade.send({...viewEvent,view:{...viewEvent.view,page:2,pages:1}})).toThrow('office-invalid-message')
  const pointer={type:'pointer-actions',phase:'request',channel:'channel',operationId:'open',documentId:'doc',version:'v1',expectedChangeSequence:0,interactionId:'interaction_id',selectionToken:'selection_token'}
  mocked.facade.send(pointer);expect(port.postMessage).toHaveBeenLastCalledWith(pointer)
  expect(()=>mocked.facade.send({...pointer,x:42})).toThrow('office-invalid-message')
  const presentation={...pointer,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}]}
  port.onmessage({data:presentation});expect(events.at(-1)).toEqual(presentation)
  const pointerCount=events.length
  port.onmessage({data:pointer});expect(events.length).toBe(pointerCount)
  expect(()=>mocked.facade.send(presentation)).toThrow('office-invalid-message')
  port.onmessage({data:{...presentation,actions:[{id:'quote',label:'bad\nlabel',enabled:true}]}});expect(events.length).toBe(pointerCount)
  const rangeLease={generation:3,sequence:0,token:pointer.selectionToken}
  const formatting={changeId:'c'.repeat(64),rangeLease,formatState:{fontFamily:'Noto Sans CJK SC',fontSize:11,bold:false,italic:null,underline:true},
    labels:{fontFamily:'字体',fontSize:'字号',bold:'粗体',italic:'斜体',underline:'下划线',mixed:'混合'}}
  const formattedPresentation={...presentation,formatting}
  port.onmessage({data:formattedPresentation});expect(events.at(-1)).toEqual(formattedPresentation)
  expect(()=>mocked.facade.send(formattedPresentation)).toThrow('office-invalid-message')
  const formatChoice={...pointer,phase:'format',changeId:formatting.changeId,rangeLease,format:{kind:'bold',value:true}}
  mocked.facade.send(formatChoice);expect(port.postMessage).toHaveBeenLastCalledWith(formatChoice)
  const formattedCount=events.length
  port.onmessage({data:formatChoice});expect(events.length).toBe(formattedCount)
  for(const next of [{...formattedPresentation,formatting:undefined},
    {...formattedPresentation,formatting:{...formatting,rangeLease:{...rangeLease,token:'other_selection'}}},
    {...formattedPresentation,expectedChangeSequence:1},
    {...formattedPresentation,formatting:{...formatting,labels:{...formatting.labels,mixed:''}}},
    {...formattedPresentation,formatting:{...formatting,labels:{...formatting.labels,mixed:'bad\nlabel'}}},
    {...formattedPresentation,formatting:{...formatting,uno:'.uno:Save'}}]) {
    port.onmessage({data:next});expect(events.length).toBe(formattedCount)
  }
  const formatSendCount=port.postMessage.mock.calls.length
  for(const next of [{...formatChoice,rangeLease:{...rangeLease,token:'other_selection'}},
    {...formatChoice,rangeLease:{...rangeLease,sequence:1}}, {...formatChoice,changeId:'change'},
    {...formatChoice,actionId:null}, {...formatChoice,format:undefined},
    {...formatChoice,format:{kind:'uno',value:'.uno:Save'}},
    {...formatChoice,format:{kind:'bold',value:true,property:'CharWeight'}},
    {...pointer,formatting}, {...pointer,phase:'choice',actionId:'quote',format:formatChoice.format}]) {
    expect(()=>mocked.facade.send(next)).toThrow('office-invalid-message')
    expect(port.postMessage.mock.calls.length).toBe(formatSendCount)
  }
  expect(Object.keys(mocked.facade).sort()).toEqual(['connect', 'send'])
})
