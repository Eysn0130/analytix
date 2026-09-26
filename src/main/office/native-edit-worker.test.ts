import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { expect, test, vi } from 'vitest'

function worker(text = '重复文本', kind = 'docx', cellType = 2, portionType = 'Text', hyperlink = '') {
  let selected = 1, readonly = true, modified = false, listener: any
  const paragraphs = ['重复文本', text]
  const messages: any[] = [], properties: any[][] = []
  const enumerate = (items: any[]) => { let i=0; return {hasMoreElements:()=>i<items.length,nextElement:()=>items[i++]} }
  const range = (i: number): any => ({
    createEnumeration: () => enumerate([{createEnumeration:()=>enumerate([{getPropertyValue:(name:string)=>name==='TextPortionType'?portionType:hyperlink}])}]),
    getString: () => paragraphs[i], setString: vi.fn((text: string) => { paragraphs[i] = text; modified = true; listener.modified() }),
    getPropertySetInfo: () => ({ getPropertyByName: () => ({ Type: 'property' }), hasPropertyByName: () => true }),
    setPropertyValue: vi.fn(() => { modified = true; listener.modified() })
  })
  const ranges = [range(0), range(1)]
  const controller: any = { getSelection: () => ({getCount: () => 1, getByIndex: () => ranges[selected]}), getFrame: () => ({getContainerWindow: () => ({}), LayoutManager:{setVisible(){},isVisible:()=>false}}), addSelectionChangeListener(){}, removeSelectionChangeListener(){} }
  const sheet = {getName:()=> 'Sheet1',getCellByPosition:()=>ranges[selected],getRows:()=>({getByIndex:()=>({getPropertyValue:()=>true})}),getColumns:()=>({getByIndex:()=>({getPropertyValue:()=>true})})}
  for (const cell of ranges) Object.assign(cell,{getType:()=>cellType,getFormula:()=>cellType === 3 ? '=SUM(A1:A2)' : '',getValue:()=>123,getPropertyValue:()=>0,getIsMerged:()=>false})
  if (kind === 'xlsx') controller.getSelection = () => ({getRangeAddress:()=>({Sheet:0,StartColumn:0,EndColumn:0,StartRow:0,EndRow:0})})
  const typography = { western: 11, asian: 11 }
  const bodyPortion = { getPropertyValue:(name:string)=>name==='CharHeight'?typography.western:typography.asian }
  const body = {createEnumeration:()=>enumerate([{createEnumeration:()=>enumerate([bodyPortion])}])}
  const model = {getText:()=>body,getSheets:()=>({getByIndex:()=>sheet}),isReadonly:()=>readonly,isModified:()=>modified,setModified:(v:boolean)=>{modified=v},getCurrentController:()=>controller,addModifyListener:(l:any)=>{listener=l},removeModifyListener(){},close(){},storeToURL:vi.fn()}
  const css = {frame:{Desktop:{create:()=>({loadComponentFromURL: (_url:any,_target:any,_flags:any,p:any[])=>{properties.push(p);readonly=p.find(v=>v.Name==='ReadOnly').Value; return model}})}},beans:{PropertyValue:function(this:any,p:any){Object.assign(this,p)}},util:{XModifyListener:{}},view:{XSelectionChangeListener:{}}}
  const port:any={postMessage:(v:any)=>messages.push(v)}
  const zeta={sameUnoObject:(a:any,b:any)=>a===b,uno:{com:{sun:{star:css}}},getUnoComponentContext(){},unoObject:(_:any,v:any)=>v,mainPort:port,Any:function(this:any,_:any,v:any){this.value=v}}
  vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js',import.meta.url),'utf8'),{Module:{zetajs:{then:(f:any)=>f(zeta)}}})
  let op=0, revision='a'.repeat(64)
  const send=(command:string,extra:any={})=>{port.onmessage({data:{command,channel:'test-channel',operationId:`operation_${++op}`,documentId:'d'.repeat(64),version:revision,...extra}});return messages.at(-1)}
  send('bind');send('open',{kind})
  return {send,paragraphs,properties,model,ranges,typography,body,select:(i:number)=>{selected=i},change:()=>{modified=true;listener.modified()},revision:(value:string)=>{revision=value}}
}
test.each([1,3])('never coerces native numeric/formula cell type %s through text replacement', cellType => {
  const w=worker('123','xlsx',cellType); w.send('edit')
  const captured=w.send('captureSelection')
  expect(captured.selection.cells[0].valueType).toBe(cellType === 1 ? 'number' : 'formula')
  expect(w.send('replace',{selectionToken:captured.selection.token,expectedChangeSequence:0,text:'变成文本',valueType:'text'})).toMatchObject({ok:false,error:'unsupported-selection'})
  expect(w.paragraphs[1]).toBe('123')
  expect(w.model.isModified()).toBe(false)
})
test.each(['docx', 'xlsx'])('unchanged %s text is refused before native mutation or export', kind => {
  const w = worker('same text', kind); w.send('edit')
  const captured = w.send('captureSelection')
  expect(w.send('replace', {selectionToken: captured.selection.token, expectedChangeSequence: 0, text: 'same text', valueType: 'text'})).toMatchObject({ok: false, error: 'invalid-control-value'})
  expect(w.ranges[1].setString).not.toHaveBeenCalled()
  expect(w.model.storeToURL).not.toHaveBeenCalled()
  expect(w.model.isModified()).toBe(false)
  expect(w.send('captureSelection')).toMatchObject({ok: true, state: {dirty: false, changeSequence: 0, acknowledgedSequence: 0}})
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
