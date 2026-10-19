import {readFileSync} from 'node:fs'
import vm from 'node:vm'
import {test,expect,vi} from 'vitest'
function fixture(count=3,editing=true){
  let listener:any,selectionListener:any,failAt=-1,formulaError=0,numberFormat=7,startRow=0,endRow=count-1,formulaResultType2:unknown=1,formulaResultType:unknown=1
  const values=Array.from({length:count},(_,i)=>({value:i+1,type:1,formula:String(i+1),text:String(i+1)})),messages:any[]=[]
  const cells=values.map((v,i)=>({getString:()=>v.text,getValue:()=>v.value,getType:()=>v.type,getFormula:()=>v.formula,getPropertyValue:(name:string)=>{
    if(name==='NumberFormat')return numberFormat
    const result=name==='FormulaResultType2'?formulaResultType2:name==='FormulaResultType'?formulaResultType:undefined
    if(result===undefined)throw Error('unavailable property')
    return result
  },getIsMerged:()=>false,getError:()=>formulaError,
    setValue:(value:number)=>{if(i===failAt)throw Error('midway');Object.assign(v,{value,type:1,formula:String(value),text:String(value)});listener.modified()},
    setFormula:(formula:string)=>{if(i===failAt)throw Error('midway');Object.assign(v,{formula,type:3,text:'3',value:3});listener.modified()},
    setString:(text:string)=>{Object.assign(v,{formula:text,type:text?2:0,text,value:0});listener.modified()}}))
  const sheet={getName:()=> 'Sheet1',getCellByPosition:(_:number,row:number)=>cells[row],getRows:()=>({getByIndex:()=>({getPropertyValue:()=>true})}),getColumns:()=>({getByIndex:()=>({getPropertyValue:()=>true})})}
  const controller={getSelection:()=>({getRangeAddress:()=>({Sheet:0,StartColumn:0,EndColumn:0,StartRow:startRow,EndRow:endRow})}),getFrame:()=>({getContainerWindow:()=>({}),LayoutManager:{setVisible(){},isVisible:()=>false}}),addSelectionChangeListener:(value:any)=>{selectionListener=value},removeSelectionChangeListener(){}}
  const model={getSheets:()=>({getByIndex:()=>sheet}),isReadonly:()=>true,getCurrentController:()=>controller,addModifyListener:(v:any)=>{listener=v},removeModifyListener(){},close(){},calculateAll:vi.fn(),storeToURL:vi.fn()}
  const css={frame:{Desktop:{create:()=>({loadComponentFromURL:()=>model})}},beans:{PropertyValue:function(this:any,v:any){Object.assign(this,v)}},util:{XModifyListener:{}},view:{XSelectionChangeListener:{}}}
  const port:any={postMessage:(v:any)=>messages.push(v)},zeta={uno:{com:{sun:{star:css}}},getUnoComponentContext(){},unoObject:(_:any,v:any)=>v,mainPort:port}
  vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js',import.meta.url),'utf8'),{Module:{zetajs:{then:(f:any)=>f(zeta)}}})
  let id=0;const send=(command:string,extra:any={})=>{port.onmessage({data:{command,channel:'channel',operationId:`operation_${++id}`,documentId:'d'.repeat(64),version:'a'.repeat(64),...extra}});return messages.at(-1)}
  send('bind');send('open',{kind:'xlsx'});if(editing)send('edit');const captured=send('captureSelection').selection
  const before=structuredClone({...captured.ranges[0],cells:captured.cells}),after=structuredClone(before)
  const apply=()=>send('replaceCells',{selectionToken:captured.token,expectedChangeSequence:0,workbook:{before,after,results:after.cells.map(()=> '')}})
  return {send,before,after,apply,values,model,select:(start:number,end=start)=>{startRow=start;endRow=end;selectionListener?.selectionChanged()},reformat:()=>{numberFormat=8},fail:(i:number)=>{failAt=i},formulaError:()=>{formulaError=532},formulaResult:(modern:unknown,legacy:unknown)=>{formulaResultType2=modern;formulaResultType=legacy}}
}
test('typed number, formula and rectangle use native typed methods and preserve numeric text protection',()=>{
  const one=fixture(1);one.after.cells[0].value=4;one.after.cells[0].formula='4';one.after.cells[0].text='4';expect(one.apply().ok).toBe(true);expect(one.values[0].type).toBe(1)
  const formula=fixture(1);Object.assign(formula.after.cells[0],{valueType:'formula',formula:'=1+2',value:0,text:''});expect(formula.apply().ok).toBe(true);expect(formula.values[0].type).toBe(3)
  const range=fixture();Object.assign(range.after.cells[0],{value:4,text:'4',formula:'4'});Object.assign(range.after.cells[2],{valueType:'formula',formula:'=SUM(A1:A2)',value:0,text:''});expect(range.apply().ok).toBe(true);expect(range.model.calculateAll).toHaveBeenCalledOnce();expect(range.send('export').ok).toBe(true)
})
test('same displayed value with changed native type or number format cannot reuse typed selection',()=>{
  const w=fixture(1);w.values[0].type=3;w.values[0].formula='=1';expect(w.apply()).toMatchObject({ok:false,error:'stale-selection'});expect(w.model.calculateAll).not.toHaveBeenCalled()
  const formatted=fixture(1);formatted.reformat();expect(formatted.apply()).toMatchObject({ok:false,error:'stale-selection'})
})
test('scope escape and unknown/external/circular formula are rejected before any mutation',()=>{
 for(const formula of ['=A4','=SUM(A1:A3)','=WEBSERVICE("https://invalid.example")','=[other]Sheet1!A1']){const w=fixture();Object.assign(w.after.cells[2],{valueType:'formula',formula});expect(w.apply().ok).toBe(false);expect(w.values[0].value).toBe(1);expect(w.model.calculateAll).not.toHaveBeenCalled()}
})
test('partial mutation and native formula error poison export until the Core original is reopened',()=>{
 for(const mode of ['throw','calculation']){const w=fixture();Object.assign(w.after.cells[0],{value:4,text:'4',formula:'4'});Object.assign(w.after.cells[2],{valueType:'formula',formula:'=SUM(A1:A2)',value:0,text:''});if(mode==='throw')w.fail(2);else w.formulaError();expect(w.apply()).toMatchObject({ok:false,error:'typed-mutation-failed'});expect(w.values[0].value).toBe(4);expect(w.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'});expect(w.model.storeToURL).not.toHaveBeenCalled()}
})

test('native formula separator conversion preserves quoted commas',()=>{
 const w=fixture(1);Object.assign(w.after.cells[0],{valueType:'formula',formula:'=IF(1,"a,b",ROUND(1.5,0))',value:0,text:''})
 expect(w.apply().ok).toBe(true)
 expect(w.values[0].formula).toBe('=IF(1;"a,b";ROUND(1.5;0))')
})

test.each(['text','empty'] as const)('typed %s replaces a numeric cell without coercing the requested type',type=>{
  const w=fixture(1)
  Object.assign(w.after.cells[0],{valueType:type,text:type==='text'?'00123':'',formula:type==='text'?'00123':'',value:0})
  expect(w.apply()).toMatchObject({ok:true,state:{dirty:true}})
  expect(w.values[0]).toMatchObject({type:type==='text'?2:0,text:type==='text'?'00123':'',value:0})
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
  const request={selectionToken:original.token,expectedChangeSequence:0,writeTarget:target,workbook:{before,after,results:['','','']}}
  expect(w.send('replaceCells',request)).toMatchObject({ok:false,error:'unsupported-command'})
  w.send('edit')
  expect(w.send('replaceCells',request)).toMatchObject({ok:true,state:{dirty:true}})
  expect(w.values.map(v=>v.formula)).toEqual(['1','2','=SUM(A1:A2)'])
  expect(w.model.isReadonly()).toBe(true)
})

test('dependency capture cannot expand writes, escape the sheet, exceed 256 cells or survive a changed selected target',()=>{
  const w=fixture(257);w.select(2)
  const original=w.send('captureSelection').selection,base={selectionToken:original.token,expectedChangeSequence:0}
  for(const bounds of [{sheet:1,startColumn:0,endColumn:0,startRow:0,endRow:2},{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:256},{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:1}])
    expect(w.send('captureCells',{...base,bounds}).ok).toBe(false)
  const captured=w.send('captureCells',{...base,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}})
  const before=captured.workbook,after=structuredClone(before)
  Object.assign(after.cells[0],{value:7,text:'7',formula:'7'})
  expect(w.send('replaceCells',{...base,writeTarget:{sheet:0,column:0,row:2},workbook:{before,after,results:['','','']}})).toMatchObject({ok:false,error:'unsupported-selection'})
  w.select(1)
  expect(w.send('captureCells',{...base,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}})).toMatchObject({ok:false,error:'stale-selection'})
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
  expect(w.send('replaceCells',{...base,writeTarget:{sheet:0,column:0,row:2},workbook:{before:captured.workbook,after,results:['','','']}})).toMatchObject({ok:false,error:'stale-selection'})
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
