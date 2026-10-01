import {readFileSync} from 'node:fs'
import vm from 'node:vm'
import {test,expect,vi} from 'vitest'
function fixture(count=3){
  let listener:any,failAt=-1,formulaError=0,numberFormat=7
  const values=Array.from({length:count},(_,i)=>({value:i+1,type:1,formula:String(i+1),text:String(i+1)})),messages:any[]=[]
  const cells=values.map((v,i)=>({getString:()=>v.text,getValue:()=>v.value,getType:()=>v.type,getFormula:()=>v.formula,getPropertyValue:()=>numberFormat,getIsMerged:()=>false,getError:()=>formulaError,
    setValue:(value:number)=>{if(i===failAt)throw Error('midway');Object.assign(v,{value,type:1,formula:String(value),text:String(value)});listener.modified()},
    setFormula:(formula:string)=>{if(i===failAt)throw Error('midway');Object.assign(v,{formula,type:3,text:'3',value:3});listener.modified()},
    setString:(text:string)=>{Object.assign(v,{formula:text,type:2,text,value:0});listener.modified()}}))
  const sheet={getName:()=> 'Sheet1',getCellByPosition:(_:number,row:number)=>cells[row],getRows:()=>({getByIndex:()=>({getPropertyValue:()=>true})}),getColumns:()=>({getByIndex:()=>({getPropertyValue:()=>true})})}
  const controller={getSelection:()=>({getRangeAddress:()=>({Sheet:0,StartColumn:0,EndColumn:0,StartRow:0,EndRow:count-1})}),getFrame:()=>({getContainerWindow:()=>({}),LayoutManager:{setVisible(){},isVisible:()=>false}}),addSelectionChangeListener(){},removeSelectionChangeListener(){}}
  const model={getSheets:()=>({getByIndex:()=>sheet}),isReadonly:()=>true,getCurrentController:()=>controller,addModifyListener:(v:any)=>{listener=v},removeModifyListener(){},close(){},calculateAll:vi.fn(),storeToURL:vi.fn()}
  const css={frame:{Desktop:{create:()=>({loadComponentFromURL:()=>model})}},beans:{PropertyValue:function(this:any,v:any){Object.assign(this,v)}},util:{XModifyListener:{}},view:{XSelectionChangeListener:{}}}
  const port:any={postMessage:(v:any)=>messages.push(v)},zeta={uno:{com:{sun:{star:css}}},getUnoComponentContext(){},unoObject:(_:any,v:any)=>v,mainPort:port}
  vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js',import.meta.url),'utf8'),{Module:{zetajs:{then:(f:any)=>f(zeta)}}})
  let id=0;const send=(command:string,extra:any={})=>{port.onmessage({data:{command,channel:'channel',operationId:`operation_${++id}`,documentId:'d'.repeat(64),version:'a'.repeat(64),...extra}});return messages.at(-1)}
  send('bind');send('open',{kind:'xlsx'});send('edit');const captured=send('captureSelection').selection
  const before=structuredClone({...captured.ranges[0],cells:captured.cells}),after=structuredClone(before)
  const apply=()=>send('replaceCells',{selectionToken:captured.token,expectedChangeSequence:0,workbook:{before,after,results:after.cells.map(()=> '')}})
  return {send,before,after,apply,values,model,reformat:()=>{numberFormat=8},fail:(i:number)=>{failAt=i},formulaError:()=>{formulaError=532}}
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
