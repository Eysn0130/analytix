import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { expect, test, vi } from 'vitest'

const inputAssetVersion = 'efaf0670b4d055f838a2849becb10f08aa06a257:5143e5354f470b87f86ba272bcfef857bd13e6f07b59666e48a7ccb89643cd77:9ebd9a487e849a24b9c69f843ebdb451709c27b7722c010e36846433474a5bd4'
function worker(text = '重复文本', kind = 'docx', cellType = 2, portionType = 'Text', hyperlink = '', comparable = false, inputOptions: {bind?:boolean;wrappers?:boolean} = {}) {
  let selected = 1, readonly = true, modified = false, listener: any, selectionListener:any, caret:number|null=null, nativeStart:number|null=null, nativeEnd:number|null=null, notifyOnAttach=false
  const paragraphs = ['重复文本', text]
  const rangeStride = Math.max(100, text.length + 1)
  const messages: any[] = [], properties: any[][] = [], lifecycle:string[] = [], characterWrites:any[] = []
  const nativeProperties:any = {CharFontName:'Noto Sans CJK SC',CharFontNameAsian:'Noto Sans CJK SC',CharFontNameComplex:'Noto Sans CJK SC',CharHeight:11,CharHeightAsian:11,CharHeightComplex:11,CharWeight:100,CharWeightAsian:100,CharWeightComplex:100,CharPosture:0,CharPostureAsian:0,CharPostureComplex:0,CharUnderline:0}
  const propertyStates:any = {}
  const nativeQueue: Array<() => void> = [], idleQueue: Array<() => void> = [], callbacks: Array<{callback:any;data:any}> = []
  const timers = new Map<number,{callback:() => void;delay:number}>(); let timerId = 0, nonceId = 0
  const runtime = {threadId:271828,
    mailbox:vi.fn(() => { for(const callback of nativeQueue.splice(0)) callback() }),
    drain:vi.fn(() => { for(const callback of idleQueue.splice(0)) callback() })}
  const runTimers = (delay:number) => { for(const [id,timer] of [...timers]) if(timer.delay === delay) { timers.delete(id);timer.callback() } }
  const notify = (index=0, value=callbacks[index].data) => callbacks[index].callback.notify(value)
  const setCharacterProperty = vi.fn((name:string,value:any) => {
    nativeProperties[name]=value.value
    delete propertyStates[name]
    if(name==='CharHeight')typography.western=value.value
    if(name==='CharHeightAsian')typography.asian=value.value
    modified=true;listener.modified()
  })
  const enumerate = (items: any[]) => { let i=0; return {hasMoreElements:()=>i<items.length,nextElement:()=>items[i++]} }
  // Native endpoints remain collapsed XTextRange objects when passed directly
  // to compareRegionStarts/Ends; an offset alone does not model that contract.
  const endpoint = (offset:number):any => {
    const point:any = {offset,getText:()=>body,getStart:()=>point,getEnd:()=>point,getString:()=>''}
    return point
  }
  const range = (i: number): any => ({
    getText: () => body, getStart: () => endpoint(i*rangeStride), getEnd: () => endpoint(i*rangeStride+paragraphs[i].length),
    createEnumeration: () => enumerate([{createEnumeration:()=>enumerate([{getPropertyValue:(name:string)=>name==='TextPortionType'?portionType:hyperlink}])}]),
    getString: () => paragraphs[i], setString: vi.fn((text: string) => { paragraphs[i] = text; modified = true; listener.modified() }),
    getPropertySetInfo: () => ({ getPropertyByName: () => ({ Type: 'property' }), hasPropertyByName: () => true }),
    getPropertyState:(name:string)=>propertyStates[name]??0,
    setPropertyValue: setCharacterProperty
  })
  const ranges = [range(0), range(1)]
  const cursor:any = {
    getText:()=>body,getStart:()=>endpoint(selected*rangeStride+(caret??nativeStart??0)),getEnd:()=>endpoint(selected*rangeStride+(caret??nativeEnd??paragraphs[selected].length)),
    getString:()=>paragraphs[selected].slice(caret??nativeStart??0,caret??nativeEnd??paragraphs[selected].length),getPropertySetInfo:()=>ranges[selected].getPropertySetInfo(),
    getPropertyValue:(name:string)=>nativeProperties[name],getPropertyState:(name:string)=>propertyStates[name]??0,setPropertyValue:setCharacterProperty,
    gotoRange:vi.fn((position:any,_expand:boolean)=>{caret=position.offset-selected*rangeStride;nativeStart=nativeEnd=null}),
    collapseToEnd:vi.fn(()=>{caret=insertedEnd;nativeStart=nativeEnd=null}),isCollapsed:()=>caret!==null
  }
  let insertedEnd=0
  const containerWindow:any = {}
  const controller: any = { getSelection: () => ({getCount: () => 1, getByIndex: () => caret===null?ranges[selected]:cursor}), getViewCursor:()=>cursor,
    getViewData:()=>({selected,caret}),restoreViewData:vi.fn((data:any)=>{selected=data.selected;caret=data.caret}),
    getFrame: () => ({getContainerWindow: () => containerWindow, LayoutManager:{setVisible(){},isVisible:()=>false}}),
    addSelectionChangeListener(value:any){selectionListener=value;if(notifyOnAttach)value.selectionChanged()}, removeSelectionChangeListener(){lifecycle.push('detach-selection')} }
  const sheet = {getName:()=> 'Sheet1',getCellByPosition:()=>ranges[selected],getRows:()=>({getByIndex:()=>({getPropertyValue:()=>true})}),getColumns:()=>({getByIndex:()=>({getPropertyValue:()=>true})})}
  for (const cell of ranges) Object.assign(cell,{getType:()=>cellType,getFormula:()=>cellType === 3 ? '=SUM(A1:A2)' : '',getValue:()=>123,getPropertyValue:(name:string)=>kind==='docx'?(nativeProperties[name]??0):0,getIsMerged:()=>false})
  if (kind === 'xlsx') controller.getSelection = () => ({getRangeAddress:()=>({Sheet:0,StartColumn:0,EndColumn:0,StartRow:0,EndRow:0})})
  const typography = { western: 11, asian: 11 }
  const bodyPortion = { getPropertyValue:(name:string)=>name==='CharHeight'?typography.western:typography.asian }
  const body:any = {createEnumeration:()=>enumerate([{createEnumeration:()=>enumerate([bodyPortion])}]),
    insertString:vi.fn((target:any,value:string,_absorb:boolean)=>{
      const start=target.getStart().offset-selected*rangeStride,end=target.getEnd().offset-selected*rangeStride
      paragraphs[selected]=paragraphs[selected].slice(0,start)+value+paragraphs[selected].slice(end);insertedEnd=start+value.length
      // Model Writer's UTF-16 count through code-point goLeft, which can
      // overselect a preceding character when the inserted text has emoji.
      const prefix=paragraphs[selected].slice(0,insertedEnd)
      nativeStart=prefix.length-Array.from(prefix).slice(-value.length).join('').length;nativeEnd=insertedEnd;caret=null
      modified=true;listener.modified()
    })}
  if (comparable) Object.assign(body, {
    queryInterface: () => body,
    compareRegionStarts: (a:any,b:any) => Math.sign(b.getStart().offset-a.getStart().offset),
    compareRegionEnds: (a:any,b:any) => Math.sign(b.getEnd().offset-a.getEnd().offset),
    createTextCursorByRange: (position:any) => {
      let point=position.offset,mark:number|null=null
      const start=()=>mark===null?point:Math.min(point,mark),end=()=>mark===null?point:Math.max(point,mark)
      return {getText:()=>body,getStart:()=>endpoint(start()),getEnd:()=>endpoint(end()),
        gotoRange:(position:any,expand:boolean)=>{mark=expand?(mark??point):null;point=position.offset},
        goLeft:(count:number,expand:boolean)=>{mark=expand?(mark??point):null;const prefix=paragraphs[Math.floor(point/rangeStride)].slice(0,point%rangeStride),characters=Array.from(prefix);if(count>characters.length)return false;point-=characters.slice(-count).join('').length;return true},
        getString:()=>paragraphs[Math.floor(start()/rangeStride)].slice(start()%rangeStride,end()%rangeStride).replace(/\r/g,'\r\n'),
        getPropertySetInfo:()=>ranges[selected].getPropertySetInfo(),getPropertyValue:(name:string)=>nativeProperties[name],getPropertyState:(name:string)=>propertyStates[name]??0,
        setPropertyValue:(name:string,value:any)=>{characterWrites.push({name,start:start(),end:end(),value:value.value});setCharacterProperty(name,value)}}
    }
  })
  const model = {getText:()=>body,getSheets:()=>({getByIndex:()=>sheet}),isReadonly:()=>readonly,isModified:()=>modified,setModified:(v:boolean)=>{modified=v},getCurrentController:()=>controller,addModifyListener:(l:any)=>{listener=l;if(notifyOnAttach)l.modified()},removeModifyListener(){lifecycle.push('detach-modify')},close:vi.fn(()=>{lifecycle.push('close')}),storeToURL:vi.fn()}
  const css = {frame:{Desktop:{create:()=>({loadComponentFromURL: (_url:any,_target:any,_flags:any,p:any[])=>{properties.push(p);readonly=p.find(v=>v.Name==='ReadOnly').Value;lifecycle.push(`load:${readonly}`); return model}})}},beans:{PropertyValue:function(this:any,p:any){Object.assign(this,p)}},util:{XModifyListener:{}},view:{XSelectionChangeListener:{}},awt:{FontSlant:{NONE:{value:0},ITALIC:{value:2}},XCallback:{},Toolkit:{create:()=>({processEventsToIdle:runtime.drain})},AsyncCallback:{create:()=>({addCallback:(callback:any,data:any)=>callbacks.push({callback,data})})}}}
  const port:any={postMessage:(v:any)=>messages.push(v)}
  const zeta={sameUnoObject:(a:any,b:any)=>a===b,uno:{com:{sun:{star:css}}},getUnoComponentContext(){},unoObject:(_:any,v:any)=>v,mainPort:port,type:{string:'string'},getAnyType:(value:any)=>({toString:()=>value.type}),fromAny:(value:any)=>value.value,Any:function(this:any,type:any,v:any){this.type=type;this.value=v}}
  const inputGlobals:any = {
    Module:{zetajs:{then:(f:any)=>f(zeta)}},TextEncoder,
    ...(inputOptions.wrappers === false ? {} : {__emscripten_check_mailbox:runtime.mailbox,_pthread_self:()=>runtime.threadId}),
    crypto:{randomUUID:()=>`synthetic-nonce-${++nonceId}`},
    setTimeout:(callback:() => void,delay:number)=>{const id=++timerId;timers.set(id,{callback,delay});return id},
    clearTimeout:(id:number)=>timers.delete(id)
  }
  vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js',import.meta.url),'utf8'),inputGlobals)
  const replaceWrapper=(name:'mailbox'|'self')=>{inputGlobals[name==='mailbox'?'__emscripten_check_mailbox':'_pthread_self']=name==='mailbox'?()=>{}:()=>runtime.threadId}
  let op=0, revision='a'.repeat(64)
  const request=(command:string,extra:any={})=>{
    const data={command,channel:'test-channel',operationId:`operation_${++op}`,documentId:'d'.repeat(64),version:revision,...extra}
    const start=messages.length, completion=port.onmessage({data})
    return {completion,result:()=>messages.slice(start).find(message=>message.type==='result'&&message.operationId===data.operationId&&message.command===command)}
  }
  const send=(command:string,extra:any={})=>request(command,extra).result()
  send('bind');send('open',{kind})
  if(inputOptions.bind !== false) send('bind-input-runtime',{threadId:runtime.threadId,assetVersion:inputAssetVersion})
  return {send,request,replaceWrapper,runtime,containerWindow,nativeQueue,idleQueue,callbacks,notify,runTimers,paragraphs,properties,model,ranges,typography,body,messages,controller,cursor,nativeProperties,propertyStates,lifecycle,characterWrites,notifyOnAttach:()=>{notifyOnAttach=true},confirm:()=>selectionListener.selectionChanged(),select:(i:number)=>{selected=i;caret=nativeStart=nativeEnd=null},placeCaret:(offset:number)=>{caret=offset;nativeStart=nativeEnd=null},change:()=>{modified=true;listener.modified()},revision:(value:string)=>{revision=value}}
}
test.each([1,3])('never coerces native numeric/formula cell type %s through text replacement', cellType => {
  const w=worker('123','xlsx',cellType); w.send('edit')
  const captured=w.send('captureSelection')
  expect(captured.selection.cells[0].valueType).toBe(cellType === 1 ? 'number' : 'formula')
  expect(w.send('replace',{selectionToken:captured.selection.token,expectedChangeSequence:0,text:'变成文本',valueType:'text'})).toMatchObject({ok:false,error:'unsupported-command'})
  expect(w.paragraphs[1]).toBe('123')
  expect(w.model.isModified()).toBe(false)
})
test('range validation distinguishes identical DOCX positions and keeps only the current endpoint token eligible',()=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  const captured=w.send('captureSelection').selection
  const bound={selectionToken:captured.token,expectedChangeSequence:0}
  expect(w.send('validateSelection',bound)).toMatchObject({ok:true,state:{changeSequence:0,dirty:false}})
  expect(w.send('captureSelection').selection.token).toBe(captured.token)
  w.select(0)
  expect(w.send('validateSelection',bound)).toMatchObject({ok:false,error:'stale-selection'})
  const other=w.send('captureSelection').selection
  expect(other.token).not.toBe(captured.token)
  w.select(1)
  const returned=w.send('captureSelection').selection
  expect(returned.token).not.toBe(captured.token)
  expect(returned.token).not.toBe(other.token)
  expect(w.send('validateSelection',bound)).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('validateSelection',{selectionToken:returned.token,expectedChangeSequence:0}).ok).toBe(true)
  expect(w.model.isReadonly()).toBe(true)
  expect(w.model.isModified()).toBe(false)
  expect(w.ranges[1].setString).not.toHaveBeenCalled()
})
test('a native range object that moves to identical text cannot pass its independent captured endpoints',()=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  const captured=w.send('captureSelection').selection
  w.ranges[1].getStart=w.ranges[0].getStart;w.ranges[1].getEnd=w.ranges[0].getEnd
  expect(w.send('validateSelection',{selectionToken:captured.token,expectedChangeSequence:0})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.paragraphs).toEqual(['重复文本','重复文本'])
})
test('range comparison is optional for existing frozen operations and unavailable validation fails closed',()=>{
  const w=worker();const captured=w.send('captureSelection').selection
  expect(w.send('validateSelection',{selectionToken:captured.token,expectedChangeSequence:0})).toMatchObject({ok:false,error:'unsupported-selection'})
  expect(w.send('validateSelection',{selectionToken:captured.token,expectedChangeSequence:-1})).toMatchObject({ok:false,error:'invalid-request'})
  for(let i=0;i<32;i++)w.send('captureSelection')
  expect(w.send('validateSelection',{selectionToken:captured.token,expectedChangeSequence:0})).toMatchObject({ok:false,error:'stale-selection'})
})
test.each(['docx', 'xlsx'])('unchanged %s text is refused before native mutation or export', kind => {
  const w = worker('same text', kind); w.send('edit')
  const captured = w.send('captureSelection')
  expect(w.send('replace', {selectionToken: captured.selection.token, expectedChangeSequence: 0, text: 'same text', valueType: 'text'})).toMatchObject({ok: false, error: kind==='xlsx'?'unsupported-command':'invalid-control-value'})
  expect(w.ranges[1].setString).not.toHaveBeenCalled()
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  expect(w.model.isModified()).toBe(false)
  expect(w.send('captureSelection')).toMatchObject({ok: true, state: {dirty: false, changeSequence: 0, acknowledgedSequence: 0}})
  if(kind==='xlsx'){
    expect(w.send('replace', {selectionToken: captured.selection.token, expectedChangeSequence: 0, text: 'changed text', valueType: 'text'})).toMatchObject({ok:false,error:'unsupported-command'})
    expect(w.ranges[1].setString).not.toHaveBeenCalled()
    expect(w.model.isModified()).toBe(false)
    return
  }
  // Rejection must not consume the selection handle or invent a change.
  expect(w.send('replace', {selectionToken: captured.selection.token, expectedChangeSequence: 0, text: 'changed text', valueType: 'text'}).ok).toBe(true)
  expect(w.ranges[1].setString).toHaveBeenCalledOnce()
})
test('private AI edit preserves the readonly native model, macro prohibition and existing selection',()=>{
  const w=worker()
  expect(w.send('replace',{selectionToken:'invented',expectedChangeSequence:0,text:'x',valueType:'text'})).toMatchObject({ok:false,error:'unsupported-command'})
  const selection=w.send('captureSelection').selection
  expect(w.send('edit')).toMatchObject({ok:true,state:{changeSequence:0,dirty:false}})
  expect(w.properties).toHaveLength(1)
  expect(w.properties.map(p=>p.find(v=>v.Name==='MacroExecutionMode').Value)).toEqual([0])
  expect(w.properties.map(p=>p.find(v=>v.Name==='ReadOnly').Value)).toEqual([true])
  expect(w.properties[0].find(p=>p.Name==='LockEditDoc').Value).toBe(true)
  expect(w.model.isReadonly()).toBe(true)
  expect(w.send('replace',{selectionToken:selection.token,expectedChangeSequence:0,text:'AI受控文本',valueType:'text'}).ok).toBe(true)
  expect(w.model.isReadonly()).toBe(true)
})
test('opaque native range edits second identical text after focus moves, then rejects stale token',()=>{
  const w=worker();w.send('edit')
  const selection=w.send('captureSelection').selection
  w.select(0)
  expect(w.send('replace',{selectionToken:selection.token,expectedChangeSequence:0,text:'仅第二段',valueType:'text'}).ok).toBe(true)
  expect(w.paragraphs).toEqual(['重复文本','仅第二段'])
  expect(w.send('replace',{selectionToken:selection.token,expectedChangeSequence:0,text:'wrong',valueType:'text'})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.paragraphs).toEqual(['重复文本','仅第二段'])
})
test('ending a continuous UI lease preserves the separately captured historical AI target',()=>{
  const w=worker('重复文本','docx',2,'Text','',true);w.send('edit')
  const historical=w.send('captureSelection').selection
  w.select(0);w.confirm()
  expect(w.send('captureSelection').selection.token).not.toBe(historical.token)
  expect(w.send('validateSelection',{selectionToken:historical.token,expectedChangeSequence:0})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('replace',{selectionToken:historical.token,expectedChangeSequence:0,text:'仅第二段',valueType:'text'}).ok).toBe(true)
  expect(w.paragraphs).toEqual(['重复文本','仅第二段'])
  expect(w.ranges[0].setString).not.toHaveBeenCalled()
})
test('truncation is explicit and never splits a surrogate pair',()=>{
  const w=worker('中'.repeat(4095)+'😀尾')
  const selection=w.send('captureSelection').selection
  expect(selection.text).toBe('中'.repeat(4095))
  expect(selection.capture).toEqual({capturedCharacters:4095,totalCharacters:4098,truncated:true,unit:'utf-16',complete:false})
})
test('save acknowledgement cannot mark a later engine change saved and rejects foreign operation',()=>{
  const w=worker();w.send('edit');w.change()
  const exported=w.send('export')
  expect(exported).toMatchObject({ok:true,exportedSequence:1})
  w.change()
  const ack={status:'committed',exportOperationId:exported.operationId,exportedSequence:1,persistedVersion:'b'.repeat(64)}
  expect(w.send('ack',{...ack,exportOperationId:'other-operation'})).toMatchObject({ok:false,error:'ack-mismatch'})
  expect(w.send('ack',ack)).toMatchObject({ok:true,state:{version:'b'.repeat(64),changeSequence:2,acknowledgedSequence:1,dirty:true}})
  w.revision('b'.repeat(64))
  expect(w.send('close',{expectedChangeSequence:2,discard:false})).toMatchObject({ok:false,error:'unsaved-changes'})
})

test('manual formatting, undo, redo and numeric/formula mutation commands stay unavailable after private AI preparation',()=>{
  const w=worker();w.send('edit')
  for(const command of ['format','bold','undo','redo']) expect(w.send(command)).toMatchObject({ok:false,error:'unsupported-command'})
  const current=w.send('captureSelection')
  for(const valueType of ['number','formula']) expect(w.send('replace',{selectionToken:current.selection.token,expectedChangeSequence:0,text:'123',valueType})).toMatchObject({ok:false,error:'invalid-control-value'})
  expect(w.paragraphs).toEqual(['重复文本','重复文本'])
  expect(w.send('replace',{selectionToken:current.selection.token,expectedChangeSequence:0,text:'=SUM(A1:A2)',valueType:'text'}).ok).toBe(true)
  expect(w.paragraphs[1]).toBe('=SUM(A1:A2)')
})
test('accepted AI text replacement exports and acknowledges only the exact native revision',()=>{
  const w=worker();const captured=w.send('captureSelection');w.send('edit')
  expect(w.send('replace',{selectionToken:captured.selection.token,expectedChangeSequence:0,text:'AI修改结果',valueType:'text'})).toMatchObject({ok:true,state:{dirty:true,changeSequence:1}})
  const exported=w.send('export');expect(w.model.storeToURL).toHaveBeenCalledOnce()
  expect(w.send('ack',{status:'committed',exportOperationId:exported.operationId,exportedSequence:exported.exportedSequence,persistedVersion:'b'.repeat(64)})).toMatchObject({ok:true,state:{dirty:false,changeSequence:1,acknowledgedSequence:1}})
  expect(w.model.isReadonly()).toBe(true)
})

test.each(['docx', 'xlsx'])('a partially failed %s text mutation cannot be exported or edited again', kind => {
  const w = worker('original', kind); w.send('edit')
  const captured = w.send('captureSelection')
  w.ranges[1].setString.mockImplementation(() => {
    w.paragraphs[1] = 'partial native result'
    w.change()
    throw Error('synthetic native failure')
  })
  const failed = w.send('replace', {selectionToken: captured.selection.token, expectedChangeSequence: 0, text: 'requested result', valueType: 'text'})
  if(kind==='xlsx'){
    expect(failed).toMatchObject({ok:false,error:'unsupported-command'})
    expect(w.ranges[1].setString).not.toHaveBeenCalled()
    expect(w.paragraphs[1]).toBe('original')
    expect(w.model.isModified()).toBe(false)
    expect(w.model.storeToURL).not.toHaveBeenCalled()
    return
  }
  expect(failed.ok).toBe(false)
  expect(w.send('export')).toMatchObject({ok: false, error: 'typed-mutation-failed'})
  expect(failed.error).toBe('typed-mutation-failed')
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  const latest = w.send('captureSelection')
  expect(latest.state.dirty).toBe(true)
  expect(w.send('replace', {selectionToken: latest.selection.token, expectedChangeSequence: latest.state.changeSequence, text: 'retry', valueType: 'text'})).toMatchObject({ok: false, error: 'typed-mutation-failed'})
  expect(w.ranges[1].setString).toHaveBeenCalledOnce()
  expect(w.send('close', {expectedChangeSequence: latest.state.changeSequence, discard: false})).toMatchObject({ok: false, error: 'unsaved-changes'})
  expect(w.send('close', {expectedChangeSequence: latest.state.changeSequence, discard: true}).ok).toBe(true)
})


test('export layout notifications do not invent another edit and failure restores change tracking',()=>{
  const w=worker();w.send('edit');w.change()
  w.model.storeToURL.mockImplementation(()=>{w.change()})
  const exported=w.send('export')
  expect(exported).toMatchObject({ok:true,exportedSequence:1,state:{changeSequence:1,dirty:true}})
  expect(w.send('ack',{status:'committed',exportOperationId:exported.operationId,exportedSequence:1,persistedVersion:'b'.repeat(64)})).toMatchObject({ok:true,state:{changeSequence:1,acknowledgedSequence:1,dirty:false}})
  w.revision('b'.repeat(64))
  w.model.storeToURL.mockImplementation(()=>{w.change();throw Error('failed export')})
  expect(w.send('export').ok).toBe(false)
  w.change()
  expect(w.send('captureSelection')).toMatchObject({ok:true,state:{changeSequence:2,dirty:true}})
})


test.each([['TextField',''], ['Text','https://example.invalid/fixture'], ['Bookmark',''], ['TextContent','']])('refuses structured DOCX portion %s before setString', (portionType, link) => {
  const w=worker('before field/link', 'docx', 2, portionType, link);w.send('edit')
  const captured=w.send('captureSelection')
  expect(w.send('replace',{selectionToken:captured.selection.token,expectedChangeSequence:0,text:'replacement',valueType:'text'})).toMatchObject({ok:false,error:'unsupported-selection'})
  expect(w.ranges[1].setString).not.toHaveBeenCalled()
  expect(w.paragraphs[1]).toBe('before field/link')
  expect(w.send('captureSelection')).toMatchObject({ok:true,state:{dirty:false,changeSequence:0}})
})
test('refuses unknown DOCX enumeration instead of flattening it', () => {
  const w=worker();w.send('edit');const captured=w.send('captureSelection')
  w.ranges[1].createEnumeration=undefined
  expect(w.send('replace',{selectionToken:captured.selection.token,expectedChangeSequence:0,text:'replacement',valueType:'text'})).toMatchObject({ok:false,error:'unsupported-selection'})
  expect(w.ranges[1].setString).not.toHaveBeenCalled()
})


test('split Western/CJK body sizes refuse editing before any mutation and retain readonly preview', () => {
  const w=worker();w.typography.asian=10.5
  expect(w.send('edit')).toMatchObject({ok:false,error:'unsupported-format-fidelity'})
  expect(w.send('captureSelection')).toMatchObject({ok:true,state:{dirty:false,changeSequence:0}})
  expect(w.model.isReadonly()).toBe(true)
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  expect(w.ranges[1].setString).not.toHaveBeenCalled()
  expect(w.send('export').ok).toBe(false)
  expect(w.send('close',{expectedChangeSequence:0,discard:false}).ok).toBe(true)
})
test('rechecks typography at export instead of relying on the initial edit admission', () => {
  const w=worker();expect(w.send('edit').ok).toBe(true);w.typography.asian=10.5
  expect(w.send('export')).toMatchObject({ok:false,error:'unsupported-format-fidelity'})
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  expect(w.send('captureSelection')).toMatchObject({ok:true,state:{dirty:false,changeSequence:0}})
})
test('table-cell typography is checked as well as ordinary body paragraphs', () => {
  const w=worker();const paragraph=w.body.createEnumeration().nextElement()
  w.body.createEnumeration=()=>{let count=0;return {hasMoreElements:()=>count===0,nextElement:()=>{count++;return {getCellNames:()=>['A1'],getCellByName:()=>({createEnumeration:()=>{let n=0;return {hasMoreElements:()=>n===0,nextElement:()=>{n++;return paragraph}}}})}}}}
  w.typography.asian=10.5
  expect(w.send('edit')).toMatchObject({ok:false,error:'unsupported-format-fidelity'})
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})


test.each([NaN,Infinity,0,-1])('unknown or invalid native font size %s never permits an export', value=>{
  const w=worker();w.typography.western=value;w.typography.asian=value
  expect(w.send('edit')).toMatchObject({ok:false,error:'unsupported-format-fidelity'})
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})
test('unbounded native enumeration fails closed without hanging or mutating',()=>{
  const w=worker();const block=w.body.createEnumeration().nextElement()
  w.body.createEnumeration=()=>({hasMoreElements:()=>true,nextElement:()=>block})
  expect(w.send('edit')).toMatchObject({ok:false,error:'unsupported-format-fidelity'})
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  expect(w.send('captureSelection')).toMatchObject({ok:true,state:{dirty:false,changeSequence:0}})
})

test('native confirmation retains the current endpoint token across repeated samples and clears on mutation/close',()=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  w.confirm();const first=w.messages.at(-1)
  expect(first).toMatchObject({type:'selection-confirmed',eligible:true,changed:true})
  w.confirm();expect(w.messages.at(-1)).toMatchObject({eligible:true,changed:false})
  expect(w.messages.at(-1).selectionToken).toBe(first.selectionToken)
  for(let i=0;i<40;i++)expect(w.send('captureSelection').selection.token).toBe(first.selectionToken)
  expect(w.send('validateSelection',{selectionToken:first.selectionToken,expectedChangeSequence:0}).ok).toBe(true)
  w.select(0);w.confirm();expect(w.messages.at(-1)).toMatchObject({eligible:true,changed:true})
  expect(w.messages.at(-1).selectionToken).not.toBe(first.selectionToken)
  w.select(1);w.confirm();const returned=w.messages.at(-1).selectionToken
  expect(returned).not.toBe(first.selectionToken)
  w.change();w.confirm();expect(w.messages.at(-1)).toMatchObject({eligible:true,changed:true})
  expect(w.messages.at(-1).selectionToken).not.toBe(returned)
  expect(w.send('validateSelection',{selectionToken:first.selectionToken,expectedChangeSequence:0})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('close',{expectedChangeSequence:1,discard:true}).ok).toBe(true)
})

const manualAuthority = (changeId='c'.repeat(64),baseRevision='a'.repeat(64)) => ({
  version:2,origin:'manual',changeId,baseRevision,saveOperationId:'manual_save_0001',undoOperationId:'manual_undo_0001'
})
test('manual repeated samples retain one continuous lease through equivalent native wrappers without mutation',()=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  const first=w.send('manual-edit',{manual:manualAuthority()}).selection
  const nativeSelection=w.controller.getSelection
  w.controller.getSelection=()=>{const selected=nativeSelection();return {getCount:()=>1,getByIndex:()=>({...selected.getByIndex(0)})}}
  w.confirm()
  for(let i=0;i<40;i++) {
    const sampled=w.send('captureSelection').selection
    expect(sampled.token).toBe(first.token)
    expect(sampled.rangeLease).toEqual(first.rangeLease)
  }
  w.confirm();expect(w.messages.at(-1)).toMatchObject({type:'selection-confirmed',selectionToken:first.token,eligible:true,changed:false})
  expect(w.send('validateSelection',{selectionToken:first.token,expectedChangeSequence:0}).ok).toBe(true)
  w.select(0);const other=w.send('captureSelection').selection
  expect(other.token).not.toBe(first.token)
  w.select(1);const returned=w.send('captureSelection').selection
  expect(returned.token).not.toBe(first.token)
  expect(returned.token).not.toBe(other.token)
  expect(w.send('format',{changeId:'c'.repeat(64),rangeLease:first.rangeLease,format:{kind:'bold',value:true}})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('validateSelection',{selectionToken:returned.token,expectedChangeSequence:0}).ok).toBe(true)
  expect(w.body.insertString).not.toHaveBeenCalled()
  expect(w.cursor.setPropertyValue).not.toHaveBeenCalled()
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  expect(w.model.isModified()).toBe(false)
})
test.each(['empty','unavailable','multiple','comparison'] as const)('%s selection ends the manual continuous lease without reviving it on return',mode=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  const first=w.send('manual-edit',{manual:manualAuthority()}).selection
  const nativeSelection=w.controller.getSelection, compare=w.body.compareRegionStarts
  if(mode==='empty')w.placeCaret(0)
  else if(mode==='unavailable')w.controller.getSelection=()=>{throw Error('synthetic unavailable selection')}
  else if(mode==='multiple')w.controller.getSelection=()=>({getCount:()=>2,getByIndex:()=>w.ranges[1]})
  else w.body.compareRegionStarts=()=>{throw Error('synthetic comparison failure')}
  const interrupted=w.send('captureSelection').selection
  expect(interrupted.rangeLease).toBeUndefined()
  if(mode==='unavailable')expect(interrupted.kind).toBe('unavailable')
  w.controller.getSelection=nativeSelection;w.body.compareRegionStarts=compare;w.select(1)
  const returned=w.send('captureSelection').selection
  expect(returned.token).not.toBe(first.token)
  expect(w.send('validateSelection',{selectionToken:first.token,expectedChangeSequence:0})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('format',{changeId:'c'.repeat(64),rangeLease:first.rangeLease,format:{kind:'bold',value:true}})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('validateSelection',{selectionToken:returned.token,expectedChangeSequence:0}).ok).toBe(true)
  expect(w.paragraphs).toEqual(['重复文本','重复文本'])
  expect(w.cursor.setPropertyValue).not.toHaveBeenCalled()
})
test.each(['null','throw'] as const)('a %s native format cursor ends the continuous lease before mutation',mode=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  const first=w.send('manual-edit',{manual:manualAuthority()}).selection
  const nativeCursor=w.controller.getViewCursor
  w.controller.getViewCursor=()=>{if(mode==='throw')throw Error('synthetic unavailable view cursor');return null}
  const request={changeId:'c'.repeat(64),rangeLease:first.rangeLease,format:{kind:'bold',value:true}}
  expect(w.send('format',request)).toMatchObject({ok:false,error:'stale-selection'})
  w.controller.getViewCursor=nativeCursor
  expect(w.send('format',request)).toMatchObject({ok:false,error:'stale-selection'})
  const returned=w.send('captureSelection')
  expect(returned).toMatchObject({ok:true,state:{dirty:false,changeSequence:0}})
  expect(returned.selection.token).not.toBe(first.token)
  expect(w.send('validateSelection',{selectionToken:returned.selection.token,expectedChangeSequence:0}).ok).toBe(true)
  expect(w.body.insertString).not.toHaveBeenCalled()
  expect(w.cursor.setPropertyValue).not.toHaveBeenCalled()
  expect(w.model.isModified()).toBe(false)
})
test('equal text at equal endpoints in another native container cannot retain or revive the current lease',()=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  const first=w.send('manual-edit',{manual:manualAuthority()}).selection
  const getText=w.ranges[1].getText
  const other:any={...w.body}
  other.queryInterface=()=>other
  other.createTextCursorByRange=(position:any)=>({...w.body.createTextCursorByRange(position),getText:()=>other})
  w.ranges[1].getText=()=>other
  const moved=w.send('captureSelection').selection
  expect(moved.token).not.toBe(first.token)
  expect(moved.rangeLease).toBeDefined()
  expect(w.send('validateSelection',{selectionToken:moved.token,expectedChangeSequence:0}).ok).toBe(true)
  w.ranges[1].getText=getText
  const returned=w.send('captureSelection').selection
  expect(returned.token).not.toBe(first.token)
  expect(returned.token).not.toBe(moved.token)
  expect(w.send('format',{changeId:'c'.repeat(64),rangeLease:first.rangeLease,format:{kind:'bold',value:true}})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('validateSelection',{selectionToken:returned.token,expectedChangeSequence:0}).ok).toBe(true)
})
test('continuous lease equality uses full native text beyond the bounded public prefix',()=>{
  const prefix='中'.repeat(4096),w=worker(prefix+'甲','docx',2,'Text','',true)
  const first=w.send('captureSelection').selection
  expect(w.send('captureSelection').selection.token).toBe(first.token)
  w.paragraphs[1]=prefix+'乙'
  const changed=w.send('captureSelection').selection
  expect(changed.text).toBe(first.text)
  expect(changed.capture).toEqual(first.capture)
  expect(changed.changeSequence).toBe(first.changeSequence)
  expect(changed.token).not.toBe(first.token)
  expect(w.send('validateSelection',{selectionToken:first.token,expectedChangeSequence:0})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('validateSelection',{selectionToken:changed.token,expectedChangeSequence:0}).ok).toBe(true)
})
test('committed version and fresh manual generation each retire the same native endpoint lease',async()=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  const first=w.send('manual-edit',{manual:manualAuthority()}).selection
  const freeze=w.request('freeze',{changeId:'c'.repeat(64)})
  w.notify();w.runTimers(0);await freeze.completion
  const proof=freeze.result().freezeAck
  const exported=w.send('export',{changeId:'c'.repeat(64),freezeAck:proof})
  const ack=w.send('ack',{status:'committed',exportOperationId:exported.operationId,exportedSequence:exported.exportedSequence,persistedVersion:'b'.repeat(64)})
  expect(ack).toMatchObject({ok:true,state:{version:'b'.repeat(64),changeSequence:0,dirty:false}})
  expect(ack.selection.token).not.toBe(first.token)
  w.revision('b'.repeat(64))
  expect(w.send('captureSelection').selection.token).toBe(ack.selection.token)
  const next={...manualAuthority('e'.repeat(64),'b'.repeat(64)),saveOperationId:'manual_save_0002',undoOperationId:'manual_undo_0002'}
  const reopened=w.send('manual-edit',{manual:next})
  expect(reopened).toMatchObject({ok:true,state:{manual:{generation:2}},selection:{text:'重复文本'}})
  expect(reopened.selection.token).not.toBe(ack.selection.token)
  expect(reopened.selection.rangeLease.generation).toBe(2)
  expect(w.send('validateSelection',{selectionToken:ack.selection.token,expectedChangeSequence:0})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.properties).toHaveLength(2)
})
test('manual admission requires the exact Core authority rather than a missing proposal',()=>{
  const w=worker()
  for(const manual of [undefined,{...manualAuthority(),version:1},{...manualAuthority(),origin:'proposal'},
    {...manualAuthority(),baseRevision:'b'.repeat(64)},{...manualAuthority(),uno:'.uno:EditDoc'}]) {
    expect(w.send('manual-edit',{manual})).toMatchObject({ok:false,error:'invalid-request'})
  }
  expect(w.properties).toHaveLength(1)
  expect(w.model.close).not.toHaveBeenCalled()
  expect(w.model.isReadonly()).toBe(true)
})
test('manual admission samples actual native descriptor names once, filters them, and keeps its cached list private and independent',()=>{
  const w=worker('原生正文','docx',2,'Text','',true), request={operationId:'manual_fonts_0001',manual:manualAuthority()}
  const descriptors=[{Name:'Zeta'},{Name:'Noto Sans CJK SC'},{Name:'Alpha'},{Name:'Zeta'},
    null,{},'filename.ttf',{Name:123},{Name:''},{Name:' Bad '},{Name:'bad\0name'},{Name:'x'.repeat(129)}]
  w.containerWindow.getFontDescriptors=vi.fn(()=>descriptors)
  expect(w.send('manual-edit',{manual:{...manualAuthority(),origin:'proposal'}})).toMatchObject({ok:false,error:'invalid-request'})
  expect(w.containerWindow.getFontDescriptors).not.toHaveBeenCalled()
  const result=w.send('manual-edit',request)
  expect(result.availableFonts).toEqual(['Alpha','Noto Sans CJK SC','Zeta'])
  expect(result.inputRuntime).toEqual({assetVersion:inputAssetVersion,threadMatches:true,generation:1,changeId:'c'.repeat(64)})
  expect(result.state).toMatchObject({dirty:false,changeSequence:0})
  result.availableFonts.push('caller mutation')
  w.containerWindow.getFontDescriptors.mockReturnValue([{Name:'Other loaded list'}])
  expect(w.send('manual-edit',request).availableFonts).toEqual(['Alpha','Noto Sans CJK SC','Zeta'])
  expect(w.containerWindow.getFontDescriptors).toHaveBeenCalledOnce()
  expect(w.body.insertString).not.toHaveBeenCalled()
  expect(w.cursor.setPropertyValue).not.toHaveBeenCalled()
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})
test.each(['mailbox','self'] as const)('font descriptor callback replacing %s rejects manual capability and retires input',name=>{
  const w=worker('原生正文','docx',2,'Text','',true)
  w.containerWindow.getFontDescriptors=vi.fn(()=>{w.replaceWrapper(name);return [{Name:'Alpha'},{Name:'Zeta'}]})
  const result=w.send('manual-edit',{manual:manualAuthority()})
  expect(w.containerWindow.getFontDescriptors).toHaveBeenCalledOnce()
  expect(result).toMatchObject({ok:false,error:'native-input-unavailable'})
  expect(result.inputRuntime).toBeUndefined()
  expect(result.availableFonts).toBeUndefined()
  expect(w.send('manual-edit',{manual:manualAuthority()})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:'不得输入'})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.body.insertString).not.toHaveBeenCalled()
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})
test('missing, malformed or oversized native font capability reports no choices without weakening manual authority',()=>{
  const capabilities=[undefined,'not callable',()=>{throw Error('synthetic font query failure')},
    ()=>({Name:'Not an array'}),()=>Array.from({length:4097},()=>({Name:'Repeated'})),
    ()=>Array.from({length:257},(_,index)=>({Name:'Family '+index}))]
  for(const capability of capabilities) {
    const w=worker('原生正文','docx',2,'Text','',true)
    w.containerWindow.getFontDescriptors=capability
    expect(w.send('manual-edit',{manual:manualAuthority()})).toMatchObject({ok:true,availableFonts:[],state:{dirty:false,manual:{generation:1}}})
    expect(w.body.insertString).not.toHaveBeenCalled()
    expect(w.model.storeToURL).not.toHaveBeenCalled()
  }
})
test('font descriptor caching is cleared for a newly loaded model and accepts the exact family limit',()=>{
  const w=worker('原生正文','docx',2,'Text','',true)
  const names=Array.from({length:256},(_,index)=>'Family '+String(index).padStart(3,'0'))
  w.containerWindow.getFontDescriptors=vi.fn(()=>names.map(Name=>({Name})))
  expect(w.send('manual-edit',{manual:manualAuthority()}).availableFonts).toEqual(names)
  expect(w.send('close',{expectedChangeSequence:0,discard:true}).ok).toBe(true)
  expect(w.send('open',{kind:'docx'}).ok).toBe(true)
  w.containerWindow.getFontDescriptors.mockReturnValue([{Name:'New model font'}])
  expect(w.send('manual-edit',{manual:manualAuthority('e'.repeat(64))}).availableFonts).toEqual(['New model font'])
  expect(w.containerWindow.getFontDescriptors).toHaveBeenCalledTimes(2)
})
test('first manual admission closes the clean readonly copy before safe same-engine reload and suppresses initialization events',()=>{
  const w=worker('原生正文','docx',2,'Text','',true), before=w.messages.length
  w.notifyOnAttach()
  const result=w.send('manual-edit',{manual:manualAuthority()})
  expect(result).toMatchObject({ok:true,state:{version:'a'.repeat(64),changeSequence:0,acknowledgedSequence:0,dirty:false,
    manual:{version:2,origin:'manual',changeId:'c'.repeat(64),baseRevision:'a'.repeat(64),generation:1,frozen:false}}})
  expect(w.lifecycle).toEqual(['load:true','detach-modify','detach-selection','close','load:false'])
  expect(w.messages.slice(before).map(message=>message.type)).toEqual(['result'])
  expect(result.inputRuntime).toEqual({assetVersion:inputAssetVersion,threadMatches:true,generation:1,changeId:'c'.repeat(64)})
  // The reload handoff reuses the initial private binding; its first-bind guard
  // still forbids rebinding an already writable model.
  expect(w.send('bind-input-runtime',{threadId:w.runtime.threadId,assetVersion:inputAssetVersion})).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(w.properties.map(props=>props.find(value=>value.Name==='ReadOnly').Value)).toEqual([true,false])
  for(const props of w.properties) for(const [name,value] of Object.entries({MacroExecutionMode:0,UpdateDocMode:0,LockEditDoc:true,LockSave:true,LockExport:true,PickListEntry:false})) {
    expect(props.find(item=>item.Name===name)?.Value).toBe(value)
  }
  expect(w.model.isReadonly()).toBe(false)
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  expect(w.send('edit')).toMatchObject({ok:false,error:'unsupported-command'})
})
test.each(['mailbox','self','thread'] as const)('manual reload refuses changed %s identity after restore before granting input',changed=>{
  const w=worker('原生正文','docx',2,'Text','',true)
  w.controller.restoreViewData.mockImplementationOnce(()=>{
    if(changed==='thread') w.runtime.threadId+=4
    else w.replaceWrapper(changed)
  })
  const result=w.send('manual-edit',{manual:manualAuthority()})
  expect(result).toMatchObject({ok:false,error:'native-input-unavailable'})
  expect(result.inputRuntime).toBeUndefined()
  expect(w.properties.map(props=>props.find(value=>value.Name==='ReadOnly').Value)).toEqual([true,false])
  expect(w.send('captureSelection').state.manual).toBeUndefined()
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:'不得输入'})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.body.insertString).not.toHaveBeenCalled()
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})
test('a repeated manual begin keeps the same authority, generation and current native selection',()=>{
  const w=worker('原生正文','docx',2,'Text','',true), request={operationId:'manual_begin_0001',manual:manualAuthority()}
  expect(w.send('manual-edit',request).state.manual.generation).toBe(1)
  w.select(0)
  expect(w.send('manual-edit',request)).toMatchObject({ok:true,state:{manual:{generation:1}},selection:{text:'重复文本'},inputRuntime:{assetVersion:inputAssetVersion,threadMatches:true,generation:1,changeId:'c'.repeat(64)}})
  expect(w.properties).toHaveLength(2)
  expect(w.send('manual-edit',{manual:manualAuthority()})).toMatchObject({ok:false,error:'operation-in-progress'})
})
test('failed native close never opens a writable replacement or permits a later mutation',()=>{
  const w=worker()
  w.model.close.mockImplementation(()=>{throw Error('synthetic close veto')})
  expect(w.send('manual-edit',{manual:manualAuthority()})).toMatchObject({ok:false,error:'native-input-unavailable'})
  expect(w.properties).toHaveLength(1)
  expect(w.send('manual-edit',{manual:manualAuthority()})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.body.insertString).not.toHaveBeenCalled()
})
test('manual format uses all three script properties and renews an endpoint-bound lease for consecutive operations',()=>{
  const w=worker('原生正文','docx',2,'Text','',true)
  const opened=w.send('manual-edit',{manual:manualAuthority()}), lease=opened.selection.rangeLease
  expect(lease).toMatchObject({generation:1,sequence:0})
  expect(opened.selection.formatState).toEqual({fontFamily:'Noto Sans CJK SC',fontSize:11,bold:false,italic:false,underline:false})
  const bold=w.send('format',{changeId:'c'.repeat(64),rangeLease:lease,format:{kind:'bold',value:true}})
  expect(bold).toMatchObject({ok:true,formatState:{bold:true},state:{dirty:true,manual:{frozen:false}}})
  expect(['CharWeight','CharWeightAsian','CharWeightComplex'].map(name=>w.nativeProperties[name])).toEqual([150,150,150])
  expect(bold.rangeLease.token).not.toBe(lease.token)
  expect(bold.rangeLease.sequence).toBe(bold.state.changeSequence)
  expect(w.send('format',{changeId:'c'.repeat(64),rangeLease:lease,format:{kind:'bold',value:false}})).toMatchObject({ok:false,error:'stale-selection'})
  const italic=w.send('format',{changeId:'c'.repeat(64),rangeLease:bold.rangeLease,format:{kind:'italic',value:true}})
  expect(italic).toMatchObject({ok:true,formatState:{bold:true,italic:true}})
  expect(['CharPosture','CharPostureAsian','CharPostureComplex'].map(name=>w.nativeProperties[name].value)).toEqual([2,2,2])
  const size=w.send('format',{changeId:'c'.repeat(64),rangeLease:italic.rangeLease,format:{kind:'font-size',value:14.5}})
  expect(size).toMatchObject({ok:true,formatState:{fontSize:14.5}})
  expect(['CharHeight','CharHeightAsian','CharHeightComplex'].map(name=>w.nativeProperties[name])).toEqual([14.5,14.5,14.5])
})
test('manual leases reject equal text at a different native position and closed formats reject arbitrary properties',()=>{
  const w=worker('重复文本','docx',2,'Text','',true)
  const lease=w.send('manual-edit',{manual:manualAuthority()}).selection.rangeLease
  w.select(0)
  expect(w.send('format',{changeId:'c'.repeat(64),rangeLease:lease,format:{kind:'bold',value:true}})).toMatchObject({ok:false,error:'stale-selection'})
  w.select(1)
  expect(w.send('format',{changeId:'c'.repeat(64),rangeLease:lease,format:{kind:'CharColor',value:123}})).toMatchObject({ok:false,error:'invalid-control-value'})
  expect(w.cursor.setPropertyValue).not.toHaveBeenCalled()
})
test('mixed and unavailable native character properties read back as null rather than a guessed run',()=>{
  const w=worker('原生正文','docx',2,'Text','',true)
  w.nativeProperties.CharHeightAsian=12
  w.propertyStates.CharWeightAsian=2
  w.nativeProperties.CharFontNameComplex='Other font'
  const selection=w.send('manual-edit',{manual:manualAuthority()}).selection
  // Initial fidelity admission remains Western/CJK equal; this fixture's
  // independent properties exercise ambiguous range readback only.
  expect(selection.formatState).toEqual({fontFamily:null,fontSize:null,bold:null,italic:false,underline:false})
})
test('composition and plain paste insert only at the actual native cursor, collapse it, and preserve input attributes without normalization',()=>{
  const w=worker('选中正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()})
  const value='e\u0301😀中文'
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:value})).toMatchObject({ok:true,state:{dirty:true}})
  expect(w.body.insertString).toHaveBeenCalledWith(w.cursor,value,true)
  expect(w.cursor.collapseToEnd).toHaveBeenCalledOnce()
  expect(w.paragraphs).toEqual(['重复文本',value])
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:' 后续'}).ok).toBe(true)
  expect(w.paragraphs[1]).toBe(value+' 后续')
  expect(w.nativeProperties.CharFontNameAsian).toBe('Noto Sans CJK SC')
  expect(w.ranges[0].setString).not.toHaveBeenCalled()
  expect(w.ranges[1].setString).not.toHaveBeenCalled()
  for(const command of ['delete-backward','delete-forward','undo','redo']) expect(w.send(command)).toMatchObject({ok:false,error:'unsupported-command'})
})
test('emoji and multiline paste retain nondefault CJK format only on the exact inserted native range',()=>{
  const w=worker('前后','docx',2,'Text','',true)
  for(const name of ['CharFontName','CharFontNameAsian','CharFontNameComplex'])w.nativeProperties[name]='合成字体'
  for(const name of ['CharHeight','CharHeightAsian','CharHeightComplex'])w.nativeProperties[name]=13.5
  for(const name of ['CharWeight','CharWeightAsian','CharWeightComplex'])w.nativeProperties[name]=150
  w.typography.western=w.typography.asian=13.5
  w.send('manual-edit',{manual:manualAuthority()});w.placeCaret(1)
  const pasted='😀\r\n首行\n末行e\u0301',native='😀\r首行\r末行e\u0301'
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:pasted})).toMatchObject({ok:true,state:{dirty:true}})
  expect(w.body.insertString).toHaveBeenCalledWith(w.cursor,native,true)
  expect(w.paragraphs[1]).toBe('前'+native+'后')
  expect(w.characterWrites).toHaveLength(13)
  expect(w.characterWrites.every(write=>write.start===101&&write.end===101+native.length)).toBe(true)
  expect(w.characterWrites.filter(write=>write.name==='CharFontNameAsian')).toEqual([{name:'CharFontNameAsian',start:101,end:101+native.length,value:'合成字体'}])
  expect(w.characterWrites.find(write=>write.name==='CharHeightAsian')?.value).toBe(13.5)
  expect(w.characterWrites.find(write=>write.name==='CharWeightAsian')?.value).toBe(150)
  const insertedEnd=w.cursor.getStart()
  expect(insertedEnd.offset).toBe(101+native.length)
  expect(insertedEnd.getStart().offset).toBe(insertedEnd.offset)
  expect(insertedEnd.getEnd().offset).toBe(insertedEnd.offset)
  expect(insertedEnd.getText()).toBe(w.body)
  expect(insertedEnd.getString()).toBe('')
  expect(w.cursor.isCollapsed()).toBe(true)
})
test('dirty manual save-first selection validation checks endpoints without pretending they are persisted',()=>{
  const w=worker('正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()});w.change()
  const captured=w.send('captureSelection')
  expect(w.send('validateSelection',{selectionToken:captured.selection.token,expectedChangeSequence:captured.state.changeSequence})).toMatchObject({ok:true,state:{dirty:true,manual:{frozen:false}}})
  w.select(0)
  expect(w.send('validateSelection',{selectionToken:captured.selection.token,expectedChangeSequence:captured.state.changeSequence})).toMatchObject({ok:false,error:'stale-selection'})
})
test.each(['','\0','\uD800','\uDC00','中'.repeat(22000)])('private manual text rejects malformed or oversized input before native mutation',text=>{
  const w=worker('正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()})
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text})).toMatchObject({ok:false,error:'invalid-control-value'})
  expect(w.body.insertString).not.toHaveBeenCalled()
  expect(w.send('captureSelection').state).toMatchObject({dirty:false,changeSequence:0})
})
test('a partially failed native manual insertion poisons the model instead of exporting partial text',()=>{
  const w=worker('正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()})
  w.body.insertString.mockImplementation(()=>{w.paragraphs[1]='部分结果';w.change();throw Error('synthetic native insertion failure')})
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:'输入'})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:'重试'})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})
test('manual input requires exact private asset/thread binding before opening a writable model',()=>{
  const w=worker('正文','docx',2,'Text','',true,{bind:false})
  expect(w.send('manual-edit',{manual:manualAuthority()})).toMatchObject({ok:false,error:'native-input-unavailable'})
  for(const binding of [
    {threadId:w.runtime.threadId+1,assetVersion:inputAssetVersion},
    {threadId:w.runtime.threadId,assetVersion:'other-assets'},
    {threadId:w.runtime.threadId,assetVersion:inputAssetVersion,exportName:'arbitrary'}
  ]) expect(w.send('bind-input-runtime',binding)).toMatchObject({ok:false,error:'native-input-unavailable'})
  expect(w.model.isReadonly()).toBe(true)
  expect(w.properties).toHaveLength(1)
  expect(w.send('bind-input-runtime',{threadId:w.runtime.threadId,assetVersion:inputAssetVersion})).toMatchObject({ok:true,inputRuntime:{assetVersion:inputAssetVersion,threadMatches:true}})
  expect(w.send('manual-edit',{manual:manualAuthority()}).ok).toBe(true)
  const missing=worker('正文','docx',2,'Text','',true,{wrappers:false})
  expect(missing.send('manual-edit',{manual:manualAuthority()})).toMatchObject({ok:false,error:'native-input-unavailable'})
})
test.each(['mailbox','self'] as const)('replacing the admitted %s function retires manual input even if its type and thread value still match',name=>{
  const w=worker('正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()});w.replaceWrapper(name)
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:'拒绝'})).toMatchObject({ok:false,error:'native-input-unavailable'})
  expect(w.body.insertString).not.toHaveBeenCalled()
  expect(w.runtime.mailbox).not.toHaveBeenCalled()
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
})
test('mailbox input drains before plaintext insertion and invalidates an old formatting lease',()=>{
  const w=worker('正文','docx',2,'Text','',true)
  const opened=w.send('manual-edit',{manual:manualAuthority()})
  w.nativeQueue.push(()=>{w.paragraphs[1]='原生';w.change();w.placeCaret(2)})
  w.idleQueue.push(()=>{expect(w.runtime.mailbox).toHaveBeenCalledOnce();expect(w.body.insertString).not.toHaveBeenCalled()})
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:'输入'}).ok).toBe(true)
  expect(w.paragraphs[1]).toBe('原生输入')
  expect(w.runtime.drain).toHaveBeenCalledOnce()
  w.nativeQueue.push(()=>w.change())
  expect(w.send('format',{changeId:'c'.repeat(64),rangeLease:opened.selection.rangeLease,format:{kind:'bold',value:true}})).toMatchObject({ok:false,error:'stale-selection'})
})
test('manual selection reads drain queued input and refuse the resulting stale validation handle',()=>{
  const w=worker('正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()})
  w.nativeQueue.push(()=>w.change())
  const captured=w.send('captureSelection')
  expect(captured).toMatchObject({ok:true,state:{dirty:true,changeSequence:1}})
  w.nativeQueue.push(()=>w.change())
  expect(w.send('validateSelection',{selectionToken:captured.selection.token,expectedChangeSequence:1})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})
test('freeze binds the sequence after mailbox and idle drain, then waits for exact native notify and callback exit',async()=>{
  const w=worker('正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()})
  w.nativeQueue.push(()=>w.change());w.idleQueue.push(()=>w.change())
  const request=w.request('freeze',{operationId:'manual_freeze_0001',changeId:'c'.repeat(64)})
  expect(request.result()).toBeUndefined()
  expect(w.runtime.mailbox).toHaveBeenCalledOnce();expect(w.runtime.drain).toHaveBeenCalledOnce()
  expect(w.callbacks).toHaveLength(1)
  expect(JSON.parse(w.callbacks[0].data.value)).toMatchObject({documentId:'d'.repeat(64),version:'a'.repeat(64),changeId:'c'.repeat(64),operationId:'manual_freeze_0001',generation:1,sequence:2})
  expect(w.send('captureSelection')).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(w.send('freeze',{operationId:'manual_freeze_0001',changeId:'c'.repeat(64)})).toBeUndefined()
  w.notify();w.notify()
  expect(request.result()).toBeUndefined()
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  w.runTimers(0);await request.completion
  const proof={changeId:'c'.repeat(64),operationId:'manual_freeze_0001',generation:1,sequence:2}
  expect(request.result()).toMatchObject({ok:true,freezeAck:proof,state:{changeSequence:2,manual:{frozen:true}}})
  const replay=w.request('freeze',{operationId:'manual_freeze_0001',changeId:'c'.repeat(64)});await replay.completion
  expect(replay.result()).toMatchObject({ok:true,freezeAck:proof})
  expect(w.callbacks).toHaveLength(1)
  expect(w.send('freeze',{changeId:'c'.repeat(64)})).toMatchObject({ok:false,error:'export-awaiting-ack'})
  expect(w.send('export',{changeId:'c'.repeat(64),freezeAck:{...proof,operationId:'invented_0001'}})).toMatchObject({ok:false,error:'ack-mismatch'})
  const exported=w.send('export',{changeId:'c'.repeat(64),freezeAck:proof})
  expect(exported).toMatchObject({ok:true,exportedSequence:2})
  expect(w.send('ack',{status:'failed',exportOperationId:exported.operationId,exportedSequence:2})).toMatchObject({ok:true,state:{manual:{frozen:true}}})
  expect(w.send('freeze',{changeId:'c'.repeat(64)})).toMatchObject({ok:false,error:'export-awaiting-ack'})
  const retried=w.send('export',{changeId:'c'.repeat(64),freezeAck:proof})
  expect(retried.ok).toBe(true)
  expect(w.send('ack',{status:'committed',exportOperationId:retried.operationId,exportedSequence:2,persistedVersion:'b'.repeat(64)})).toMatchObject({ok:true,state:{dirty:false}})
  w.revision('b'.repeat(64));w.select(0)
  const next={...manualAuthority('e'.repeat(64),'b'.repeat(64)),saveOperationId:'manual_save_0002',undoOperationId:'manual_undo_0002'}
  expect(w.send('manual-edit',{manual:next})).toMatchObject({ok:true,state:{manual:{generation:2,frozen:false}},selection:{text:'重复文本'},inputRuntime:{assetVersion:inputAssetVersion,threadMatches:true,generation:2,changeId:'e'.repeat(64)}})
  expect(w.properties).toHaveLength(2)
})
test.each(['nonce','generation','typed-any','sequence','timeout'])('freeze rejects %s mismatch or expiration and retires its mutation sink',async mode=>{
  const w=worker('正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()})
  const request=w.request('freeze',{changeId:'c'.repeat(64)})
  const original=w.callbacks[0].data, payload=JSON.parse(original.value)
  if(mode==='timeout') w.runTimers(5000)
  else if(mode==='sequence') { w.change();w.notify() }
  else if(mode==='typed-any') w.notify(0,{type:'long',value:original.value})
  else { payload[mode]=mode==='nonce'?'foreign-nonce':payload.generation+1;w.notify(0,{type:'string',value:JSON.stringify(payload)}) }
  await request.completion
  expect(request.result()).toMatchObject({ok:false,error:'native-input-unavailable'})
  w.notify();w.runTimers(0)
  expect(w.send('insert-text',{changeId:'c'.repeat(64),text:'不得写入'})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.send('export',{changeId:'c'.repeat(64)})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})
test('close cancels a delayed native callback without blocking and late notify cannot freeze a new document',async()=>{
  const w=worker('正文','docx',2,'Text','',true)
  w.send('manual-edit',{manual:manualAuthority()})
  const request=w.request('freeze',{changeId:'c'.repeat(64)})
  expect(w.send('close',{expectedChangeSequence:0,discard:true}).ok).toBe(true)
  await request.completion
  expect(request.result()).toMatchObject({ok:false,error:'native-input-unavailable'})
  expect(w.send('open',{kind:'docx'}).ok).toBe(true)
  w.notify();w.runTimers(0)
  expect(w.send('captureSelection')).toMatchObject({ok:true,state:{changeSequence:0,dirty:false}})
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})
