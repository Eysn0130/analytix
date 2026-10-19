import {readFileSync} from 'node:fs'
import vm from 'node:vm'
import {test,expect,vi} from 'vitest'
function fixture(count=3,editing=true){
  let listener:any,selectionListener:any,failAt=-1,formulaError=0,numberFormat=7,startRow=0,endRow=count-1,formulaResultType2:unknown=1,formulaResultType:unknown=1,restoreMode='normal',differentDefinition=false
  const values=Array.from({length:count},(_,i)=>({value:i+1,type:1,formula:String(i+1),text:String(i+1)})),messages:any[]=[],warnings:unknown[][]=[]
  const candidateValues=structuredClone(values),candidateFormats=values.map(()=>107),changed=new Set<number>()
  class Any {constructor(readonly type:string,readonly val:number){}}
  const makeCells=(records:typeof values,candidate:boolean)=>records.map((v,i)=>({getString:()=>v.text,getValue:()=>v.value,getType:()=>v.type,getFormula:()=>v.formula,getPropertyValue:(name:string)=>{
    if(name==='NumberFormat')return candidate?candidateFormats[i]:numberFormat
    const result=name==='FormulaResultType2'?formulaResultType2:name==='FormulaResultType'?formulaResultType:undefined
    if(result===undefined)throw Error('unavailable property')
    return result
  },getIsMerged:()=>false,getError:()=>formulaError,
    getPropertySetInfo:()=>({getPropertyByName:()=>({Type:'long'})}),
    setPropertyValue:vi.fn((name:string,value:Any)=>{if(!candidate||name!=='NumberFormat'||!(value instanceof Any)||value.type!=='long')throw Error('untyped format');if(restoreMode==='throw')throw Error('private restore failure');if(restoreMode!=='ignored')candidateFormats[i]=value.val}),
    setValue:vi.fn((value:number)=>{if(i===failAt)throw Error('midway');Object.assign(v,{value,type:1,formula:String(value),text:String(value)});if(candidate){candidateFormats[i]=900;changed.add(i)}else listener.modified()}),
    setFormula:vi.fn((formula:string)=>{if(i===failAt)throw Error('midway');Object.assign(v,{formula,type:3,text:'3',value:3});if(candidate){candidateFormats[i]=900;changed.add(i)}else listener.modified()}),
    setString:vi.fn((text:string)=>{Object.assign(v,{formula:text,type:text?2:0,text,value:0});if(candidate){candidateFormats[i]=900;changed.add(i)}else listener.modified()})}))
  const cells=makeCells(values,false),candidateCells=makeCells(candidateValues,true)
  const makeSheet=(items:typeof cells)=>({getName:()=> 'Sheet1',getCellByPosition:(_:number,row:number)=>items[row],getRows:()=>({getByIndex:()=>({getPropertyValue:()=>true})}),getColumns:()=>({getByIndex:()=>({getPropertyValue:()=>true})})})
  const sheet=makeSheet(cells),candidateSheet=makeSheet(candidateCells)
  const controller={getSelection:()=>({getRangeAddress:()=>({Sheet:0,StartColumn:0,EndColumn:0,StartRow:startRow,EndRow:endRow})}),getFrame:()=>({getContainerWindow:()=>({}),LayoutManager:{setVisible(){},isVisible:()=>false}}),addSelectionChangeListener:(value:any)=>{selectionListener=value},removeSelectionChangeListener(){}}
  const formats=(candidate:boolean)=>({getByKey:(key:number)=>({getPropertyValue:(name:string)=>name==='FormatString'?(key===(candidate?107:7)?candidate&&differentDefinition?'0.00':'General':'@'):{Language:'en',Country:'US',Variant:''}})})
  const model={getSheets:()=>({getByIndex:()=>sheet}),getNumberFormats:()=>formats(false),isReadonly:()=>true,getCurrentController:()=>controller,addModifyListener:(v:any)=>{listener=v},removeModifyListener(){},close:vi.fn(),calculateAll:vi.fn(),storeToURL:vi.fn()}
  const candidateModel={getSheets:()=>({getByIndex:()=>candidateSheet}),getNumberFormats:()=>formats(true),isReadonly:()=>false,getCurrentController:vi.fn(()=>{throw Error('hidden controller unavailable')}),close:vi.fn(),calculateAll:vi.fn(()=>{for(const i of changed)candidateFormats[i]=901}),storeToURL:vi.fn()}
  const load=vi.fn((url:string)=>url==='file:///tmp/analytix-office/workbook-candidate.xlsx'?candidateModel:model)
  const css={frame:{Desktop:{create:()=>({loadComponentFromURL:load})}},beans:{PropertyValue:function(this:any,v:any){Object.assign(this,v)}},util:{XModifyListener:{}},view:{XSelectionChangeListener:{}}}
  const sameUnoObject=vi.fn((a:unknown,b:unknown)=>a===b)
  const port:any={postMessage:(v:any)=>messages.push(v)},zeta={Any,sameUnoObject,uno:{com:{sun:{star:css}}},getUnoComponentContext(){},unoObject:(_:any,v:any)=>v,mainPort:port}
  vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js',import.meta.url),'utf8'),{Module:{zetajs:{then:(f:any)=>f(zeta)}},console:{warn:(...args:unknown[])=>warnings.push(args)}})
  const candidate={changeId:'c'.repeat(64),saveOperationId:'save_operation_01',baseRevision:'a'.repeat(64)}
  let id=0;const send=(command:string,extra:any={})=>{port.onmessage({data:{command,channel:'channel',operationId:command==='export'?candidate.saveOperationId:`operation_${++id}`,documentId:'d'.repeat(64),version:'a'.repeat(64),...extra}});return messages.at(-1)}
  send('bind');send('open',{kind:'xlsx'});if(editing)send('edit');const captured=send('captureSelection').selection
  const before=structuredClone({...captured.ranges[0],cells:captured.cells}),after=structuredClone(before)
  const apply=(extra:any={})=>send('replaceCells',{candidate,selectionToken:captured.token,expectedChangeSequence:0,workbook:{before,after,results:after.cells.map(()=> '')},...extra})
  return {send,before,after,apply,values,model,candidate,candidateValues,candidateCells,candidateFormats,candidateModel,load,sameUnoObject,Any,warnings,restore:(mode:string)=>{restoreMode=mode},mismatchDefinition:()=>{differentDefinition=true},select:(start:number,end=start)=>{startRow=start;endRow=end;selectionListener?.selectionChanged()},reformat:()=>{numberFormat=8},fail:(i:number)=>{failAt=i},formulaError:()=>{formulaError=532},formulaResult:(modern:unknown,legacy:unknown)=>{formulaResultType2=modern;formulaResultType=legacy}}
}
test('typed number, formula and rectangle use native typed methods and preserve numeric text protection',()=>{
  const one=fixture(1);one.after.cells[0].value=4;one.after.cells[0].formula='4';one.after.cells[0].text='4';expect(one.apply().ok).toBe(true);expect(one.candidateValues[0].type).toBe(1);expect(one.values[0].value).toBe(1)
  const formula=fixture(1);Object.assign(formula.after.cells[0],{valueType:'formula',formula:'=1+2',value:0,text:''});expect(formula.apply().ok).toBe(true);expect(formula.candidateValues[0].type).toBe(3)
  const range=fixture();Object.assign(range.after.cells[0],{value:4,text:'4',formula:'4'});Object.assign(range.after.cells[2],{valueType:'formula',formula:'=SUM(A1:A2)',value:0,text:''});expect(range.apply().ok).toBe(true);expect(range.candidateModel.calculateAll).toHaveBeenCalledOnce();expect(range.send('export').ok).toBe(true);expect(range.model.storeToURL).not.toHaveBeenCalled()
})

test('the hidden candidate owns its format keys and typed writes while the visible workbook stays readonly',()=>{
  const w=fixture()
  Object.assign(w.after.cells[0],{valueType:'text',text:'001',formula:'001',value:0})
  Object.assign(w.after.cells[2],{valueType:'formula',text:'',formula:'=SUM(A1:A2)',value:0})
  const result=w.apply()
  expect(result).toMatchObject({ok:true,state:{changeSequence:1,dirty:true}})
  expect(result.selection).toBeUndefined()
  expect(w.candidateValues[0]).toMatchObject({type:2,text:'001'})
  expect(w.values.map(v=>v.value)).toEqual([1,2,3])
  expect(w.candidateFormats).toEqual([107,107,107])
  expect(w.before.cells[0].numberFormat).toBe(7)
  expect(w.candidateCells[0].setPropertyValue).toHaveBeenCalledExactlyOnceWith('NumberFormat',new w.Any('long',107))
  expect(w.candidateCells[1].setPropertyValue).not.toHaveBeenCalled()
  expect(w.candidateCells[2].setPropertyValue).toHaveBeenCalledExactlyOnceWith('NumberFormat',new w.Any('long',107))
  expect(w.candidateModel.getCurrentController).not.toHaveBeenCalled()
  expect(w.model.calculateAll).not.toHaveBeenCalled()
  expect(w.load).toHaveBeenLastCalledWith('file:///tmp/analytix-office/workbook-candidate.xlsx','_blank',0,expect.arrayContaining([
    expect.objectContaining({Name:'ReadOnly',Value:false}),expect.objectContaining({Name:'Hidden',Value:true}),
    expect.objectContaining({Name:'MacroExecutionMode',Value:0}),expect.objectContaining({Name:'UpdateDocMode',Value:0}),
    expect.objectContaining({Name:'LockEditDoc',Value:true}),expect.objectContaining({Name:'LockSave',Value:true}),expect.objectContaining({Name:'LockExport',Value:true})
  ]))
})

test('a candidate with a different model-local format definition is rejected before writing',()=>{
  const w=fixture(1);w.mismatchDefinition()
  Object.assign(w.after.cells[0],{valueType:'text',text:'001',formula:'001',value:0})
  expect(w.apply()).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.candidateCells[0].setString).not.toHaveBeenCalled()
  expect(w.candidateModel.calculateAll).not.toHaveBeenCalled()
  expect(w.candidateModel.close).toHaveBeenCalledOnce()
  expect(w.values[0].value).toBe(1)
})

test('a loader returning the visible native object is rejected without closing or mutating that borrowed model',()=>{
  const w=fixture(1)
  Object.assign(w.after.cells[0],{value:42,text:'42',formula:'42'})
  w.load.mockImplementationOnce(()=>w.model)
  expect(w.apply()).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.sameUnoObject).toHaveBeenCalledExactlyOnceWith(w.model,w.model)
  expect(w.model.close).not.toHaveBeenCalled()
  expect(w.candidateModel.close).not.toHaveBeenCalled()
  expect(w.values[0].value).toBe(1)
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
})

test.each(['ignored','throw'])('candidate format restoration %s is rejected without exporting or mutating the visible workbook',mode=>{
  const w=fixture(1);w.restore(mode)
  Object.assign(w.after.cells[0],{valueType:'text',text:'001',formula:'001',value:0})
  expect(w.apply()).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.values[0]).toMatchObject({type:1,value:1})
  expect(w.candidateModel.close).toHaveBeenCalledOnce()
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.candidateModel.storeToURL).not.toHaveBeenCalled()
})

test('restoring a key does not accept a changed format definition within the candidate',()=>{
  const w=fixture(1)
  Object.assign(w.after.cells[0],{valueType:'text',text:'001',formula:'001',value:0})
  w.candidateModel.calculateAll.mockImplementationOnce(()=>w.mismatchDefinition())
  expect(w.apply()).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.warnings).toEqual([['ANALYTIX_WORKBOOK_MUTATION_DIAGNOSTIC verify-format-definition:verification-mismatch']])
  expect(w.candidateModel.close).toHaveBeenCalledOnce()
})

test('Core candidate authority is required before loading a hidden writable model',()=>{
  for(const candidate of [undefined,{changeId:'c'.repeat(64),saveOperationId:'bad',baseRevision:'a'.repeat(64)},{changeId:'c'.repeat(64),saveOperationId:'save_operation_01',baseRevision:'b'.repeat(64)}]){
    const w=fixture(1)
    expect(w.apply({candidate})).toMatchObject({ok:false,error:'invalid-request'})
    expect(w.load).toHaveBeenCalledOnce()
    expect(w.candidateCells[0].setValue).not.toHaveBeenCalled()
  }
})

test('only the Core save operation exports the candidate once and ACK never returns the old visible selection',()=>{
  const w=fixture(1)
  Object.assign(w.after.cells[0],{value:42,text:'42',formula:'42'})
  expect(w.apply().ok).toBe(true)
  expect(w.send('export',{operationId:'other_operation_01'})).toMatchObject({ok:false,error:'ack-mismatch'})
  expect(w.candidateModel.storeToURL).not.toHaveBeenCalled()
  expect(w.send('export')).toMatchObject({ok:true,exportedSequence:1})
  expect(w.candidateModel.storeToURL).toHaveBeenCalledExactlyOnceWith('file:///tmp/analytix-office/output.xlsx',expect.arrayContaining([expect.objectContaining({Name:'FilterName',Value:'Calc MS Excel 2007 XML'})]))
  expect(w.candidateModel.close).toHaveBeenCalledOnce()
  expect(w.send('export')).toMatchObject({ok:false,error:'export-awaiting-ack'})
  expect(w.send('edit')).toMatchObject({ok:false,error:'export-awaiting-ack'})
  const ack=w.send('ack',{exportOperationId:w.candidate.saveOperationId,exportedSequence:1,status:'committed',persistedVersion:'b'.repeat(64)})
  expect(ack).toMatchObject({ok:true,state:{version:'b'.repeat(64),dirty:false}})
  expect(ack.selection).toBeUndefined()
  expect(w.values[0].value).toBe(1)
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})

test.each(['export','close'])('candidate %s failure poisons retries and keeps the visible workbook unchanged',phase=>{
  const w=fixture(1)
  Object.assign(w.after.cells[0],{value:42,text:'42',formula:'42'})
  expect(w.apply().ok).toBe(true)
  if(phase==='export')w.candidateModel.storeToURL.mockImplementationOnce(()=>{throw Error('private filter failure')})
  else w.candidateModel.close.mockImplementationOnce(()=>{throw Error('private close failure')})
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.candidateModel.close).toHaveBeenCalledOnce()
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.candidateModel.storeToURL).toHaveBeenCalledOnce()
  expect(w.values[0].value).toBe(1)
  expect(w.model.storeToURL).not.toHaveBeenCalled()
})

test('discard closes a prepared candidate and its readonly view without exporting',()=>{
  const w=fixture(1)
  Object.assign(w.after.cells[0],{value:42,text:'42',formula:'42'})
  expect(w.apply().ok).toBe(true)
  expect(w.send('close',{expectedChangeSequence:1,discard:false})).toMatchObject({ok:false,error:'unsaved-changes'})
  expect(w.send('close',{expectedChangeSequence:1,discard:true})).toMatchObject({ok:true})
  expect(w.candidateModel.close).toHaveBeenCalledOnce()
  expect(w.model.close).toHaveBeenCalledOnce()
  expect(w.candidateModel.storeToURL).not.toHaveBeenCalled()
})
test('same displayed value with changed native type or number format cannot reuse typed selection',()=>{
  const w=fixture(1);w.values[0].type=3;w.values[0].formula='=1';expect(w.apply()).toMatchObject({ok:false,error:'stale-selection'});expect(w.model.calculateAll).not.toHaveBeenCalled()
  const formatted=fixture(1);formatted.reformat();expect(formatted.apply()).toMatchObject({ok:false,error:'stale-selection'})
})
test('scope escape and unknown/external/circular formula are rejected before any mutation',()=>{
 for(const formula of ['=A4','=SUM(A1:A3)','=WEBSERVICE("https://invalid.example")','=[other]Sheet1!A1']){const w=fixture();Object.assign(w.after.cells[2],{valueType:'formula',formula});expect(w.apply().ok).toBe(false);expect(w.values[0].value).toBe(1);expect(w.model.calculateAll).not.toHaveBeenCalled()}
})
test('partial mutation and native formula error poison export until the Core original is reopened',()=>{
 for(const mode of ['throw','calculation']){const w=fixture();Object.assign(w.after.cells[0],{value:4,text:'4',formula:'4'});Object.assign(w.after.cells[2],{valueType:'formula',formula:'=SUM(A1:A2)',value:0,text:''});if(mode==='throw')w.fail(2);else w.formulaError();expect(w.apply()).toMatchObject({ok:false,error:'typed-mutation-failed'});expect(w.candidateValues[0].value).toBe(4);expect(w.values[0].value).toBe(1);expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'});expect(w.candidateModel.close).toHaveBeenCalledOnce();expect(w.candidateModel.storeToURL).not.toHaveBeenCalled()}
})

test('a native calculation exception after a successful write emits only its fixed stage and still poisons export',()=>{
  const w=fixture(1)
  Object.assign(w.after.cells[0],{value:4,text:'4',formula:'4'})
  w.candidateModel.calculateAll.mockImplementationOnce(()=>{throw new Error('private synthetic native exception text')})
  expect(w.apply()).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.candidateValues[0].value).toBe(4)
  expect(w.values[0].value).toBe(1)
  expect(w.warnings).toEqual([['ANALYTIX_WORKBOOK_MUTATION_DIAGNOSTIC calculate:native-exception']])
  expect(JSON.stringify(w.warnings)).not.toContain('private synthetic')
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.candidateModel.storeToURL).not.toHaveBeenCalled()
})

test('native formula separator conversion preserves quoted commas',()=>{
 const w=fixture(1);Object.assign(w.after.cells[0],{valueType:'formula',formula:'=IF(1,"a,b",ROUND(1.5,0))',value:0,text:''})
 expect(w.apply().ok).toBe(true)
 expect(w.candidateValues[0].formula).toBe('=IF(1;"a,b";ROUND(1.5;0))')
})

test.each(['text','empty'] as const)('typed %s replaces a numeric cell without coercing the requested type',type=>{
  const w=fixture(1)
  Object.assign(w.after.cells[0],{valueType:type,text:type==='text'?'00123':'',formula:type==='text'?'00123':'',value:0})
  expect(w.apply()).toMatchObject({ok:true,state:{dirty:true}})
  expect(w.candidateValues[0]).toMatchObject({type:type==='text'?2:0,text:type==='text'?'00123':'',value:0})
  expect(w.values[0]).toMatchObject({type:1,value:1})
})

test('read-only formula dependency capture keeps the original single-cell write target and native selection',()=>{
  const w=fixture(3,false);w.select(2)
  const original=w.send('captureSelection').selection,target={sheet:0,column:0,row:2}
  const captured=w.send('captureCells',{selectionToken:original.token,expectedChangeSequence:0,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}})
  expect(captured).toMatchObject({ok:true,state:{dirty:false,changeSequence:0},workbook:{startRow:0,endRow:2,cells:[{value:1},{value:2},{value:3}]}})
  expect(captured.selection).toBeUndefined()
  expect(w.send('captureSelection').selection.ranges).toEqual([{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:2,endRow:2}])
  const before=captured.workbook,after=structuredClone(before)
  Object.assign(after.cells[2],{valueType:'formula',formula:'=SUM(A1:A2)',text:'',value:0})
  const request={candidate:w.candidate,selectionToken:original.token,expectedChangeSequence:0,writeTarget:target,workbook:{before,after,results:['','','']}}
  expect(w.send('replaceCells',request)).toMatchObject({ok:false,error:'unsupported-command'})
  w.send('edit')
  expect(w.send('replaceCells',request)).toMatchObject({ok:true,state:{dirty:true}})
  expect(w.candidateValues.map(v=>v.formula)).toEqual(['1','2','=SUM(A1:A2)'])
  expect(w.values.map(v=>v.formula)).toEqual(['1','2','3'])
  expect(w.model.isReadonly()).toBe(true)
})

test.each(['load','calculate'])('a visible manual selection generation changed during native %s invalidates the candidate',phase=>{
  const w=fixture(3,false);w.select(2)
  const selected=w.send('captureSelection').selection
  const before=w.send('captureCells',{selectionToken:selected.token,expectedChangeSequence:0,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}}).workbook
  const after=structuredClone(before)
  Object.assign(after.cells[2],{valueType:'formula',formula:'=SUM(A1:A2)',text:'',value:0})
  w.send('edit')
  if(phase==='load')w.load.mockImplementationOnce(()=>{w.select(1);return w.candidateModel})
  else w.candidateModel.calculateAll.mockImplementationOnce(()=>w.select(1))
  expect(w.send('replaceCells',{candidate:w.candidate,selectionToken:selected.token,expectedChangeSequence:0,writeTarget:{sheet:0,column:0,row:2},workbook:{before,after,results:['','','']}})).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.candidateModel.close).toHaveBeenCalledOnce()
  expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'})
  expect(w.candidateModel.storeToURL).not.toHaveBeenCalled()
  if(phase==='load')expect(w.candidateCells[2].setFormula).not.toHaveBeenCalled()
  expect(w.values.map(v=>v.value)).toEqual([1,2,3])
})

test('dependency capture cannot expand writes, escape the sheet, exceed 256 cells or survive a changed selected target',()=>{
  const w=fixture(257);w.select(2)
  const original=w.send('captureSelection').selection,base={candidate:w.candidate,selectionToken:original.token,expectedChangeSequence:0}
  const captureBase={selectionToken:original.token,expectedChangeSequence:0}
  for(const bounds of [{sheet:1,startColumn:0,endColumn:0,startRow:0,endRow:2},{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:256},{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:1}])
    expect(w.send('captureCells',{...captureBase,bounds}).ok).toBe(false)
  const captured=w.send('captureCells',{...captureBase,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}})
  const before=captured.workbook,after=structuredClone(before)
  Object.assign(after.cells[0],{value:7,text:'7',formula:'7'})
  expect(w.send('replaceCells',{...base,writeTarget:{sheet:0,column:0,row:2},workbook:{before,after,results:['','','']}})).toMatchObject({ok:false,error:'unsupported-selection'})
  w.select(1)
  expect(w.send('captureCells',{...captureBase,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('replaceCells',{...base,writeTarget:{sheet:0,column:0,row:2},workbook:{before,after,results:['','','']}})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.model.calculateAll).not.toHaveBeenCalled()
  expect(w.values[0].value).toBe(1)
})

test('selection reports known formula errors instead of treating native getValue zero as a numeric result',()=>{
  const w=fixture(1);Object.assign(w.values[0],{type:3,formula:'=1/0',text:'#DIV/0!',value:0})
  expect(w.send('captureSelection').selection.cells[0]).toMatchObject({valueType:'formula',valueError:false})
  w.formulaError()
  const selected=w.send('captureSelection').selection
  expect(selected.cells[0]).toMatchObject({value:0,valueType:'formula',valueError:true})
  expect(w.send('captureCells',{selectionToken:selected.token,expectedChangeSequence:0,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:0}})).toMatchObject({ok:false,error:'unsupported-selection'})
})

test.each([
  [1,2,'number'],[2,1,'text'],[undefined,1,'number'],[undefined,2,'text'],
  [undefined,undefined,undefined],[4,1,undefined],[7,1,undefined],['1',1,undefined]
] as const)('formula result evidence uses native result properties %s / %s only', (modern,legacy,expected)=>{
  const w=fixture(1);Object.assign(w.values[0],{type:3,formula:'=IF(1,"text",0)',text:'text',value:0})
  w.formulaResult(modern,legacy)
  const selected=w.send('captureSelection').selection
  expect(selected.cells[0].valueError).toBe(false)
  expect(selected.cells[0].formulaResultType).toBe(expected)
  if(expected===undefined)expect(Object.hasOwn(selected.cells[0],'formulaResultType')).toBe(false)
  const captured=w.send('captureCells',{selectionToken:selected.token,expectedChangeSequence:0,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:0}})
  expect(captured.ok).toBe(true)
  expect(Object.hasOwn(captured.workbook.cells[0],'formulaResultType')).toBe(false)
  expect(Object.hasOwn(captured.workbook.cells[0],'valueError')).toBe(false)
})

test('leaving and returning to a selected cell invalidates only its manual dependency capture',()=>{
  const w=fixture();w.select(2)
  const selection=w.send('captureSelection').selection,base={selectionToken:selection.token,expectedChangeSequence:0}
  const captured=w.send('captureCells',{...base,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}})
  const after=structuredClone(captured.workbook);Object.assign(after.cells[2],{value:7,text:'7',formula:'7'})
  w.send('captureSelection')
  w.select(1);w.select(2)
  expect(w.send('captureCells',{...base,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}})).toMatchObject({ok:false,error:'stale-selection'})
  expect(w.send('replaceCells',{...base,candidate:w.candidate,writeTarget:{sheet:0,column:0,row:2},workbook:{before:captured.workbook,after,results:['','','']}})).toMatchObject({ok:false,error:'stale-selection'})
  // Existing Agent proposals retain their separately captured historical range.
  Object.assign(w.after.cells[0],{value:7,text:'7',formula:'7'})
  expect(w.apply().ok).toBe(true)
})

test('only a complete bounded rectangle is marked complete for selection statistics',()=>{
  const full=fixture(256).send('captureSelection').selection
  expect(full.cells).toHaveLength(256);expect(full.capture.complete).toBe(true)
  const partial=fixture(257).send('captureSelection').selection
  expect(partial.cells).toHaveLength(256);expect(partial.ranges[0].endRow).toBe(256);expect(partial.capture.complete).toBe(false)
})
