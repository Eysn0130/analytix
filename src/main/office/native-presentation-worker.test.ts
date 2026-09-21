import {readFileSync} from 'node:fs'
import vm from 'node:vm'
import {test,expect,vi} from 'vitest'
import {nativePresentationSelectionSchema,nativePresentationReviewSchema} from '../../../packages/runtime/src/contracts/native-office-editing'
import {isOfficeRequest} from './office-protocol'
function fixture(){
 let listener:any, failSize=false, selectedIndex=0
 const objects=Array.from({length:2},(_,i)=>({x:100+i*200,y:100,width:100,height:100,fill:0x123456,text:'text '+i}))
 const shapes=objects.map(v=>({getShapeType:()=> 'com.sun.star.drawing.RectangleShape',getString:()=>v.text,getName:()=>v.text,getPosition:()=>({X:v.x,Y:v.y}),getSize:()=>({Width:v.width,Height:v.height}),getPropertyState:()=>0,getPropertySetInfo:()=>({hasPropertyByName:()=>false,getPropertyByName:()=>({Type:{}})}),getPropertyValue:(k:string)=>k==='FillStyle'?1:k==='FillColor'?v.fill:0,setPropertyValue:(_:string,n:number)=>{v.fill=n;listener.modified()},setPosition:(p:any)=>{v.x=p.X;v.y=p.Y;listener.modified()},setSize:(p:any)=>{if(failSize)throw Error('partial');v.width=p.Width;v.height=p.Height;listener.modified()}}))
 const page={getCount:()=>shapes.length,getByIndex:(i:number)=>shapes[i],getPropertyValue:()=>1000}
 const pages={getCount:()=>1,getByIndex:()=>page}
 const controller={getCurrentPage:()=>page,getSelection:()=>({getCount:()=>1,getByIndex:()=>shapes[selectedIndex]}),getFrame:()=>({getContainerWindow:()=>({}),LayoutManager:{setVisible(){},isVisible:()=>false}}),addSelectionChangeListener(){},removeSelectionChangeListener(){}}
 const model={getDrawPages:()=>pages,isReadonly:()=>true,getCurrentController:()=>controller,addModifyListener:(v:any)=>{listener=v},removeModifyListener(){},close(){},storeToURL:vi.fn()}
 const struct=function(this:any,v:any){Object.assign(this,v)}
 const css={frame:{Desktop:{create:()=>({loadComponentFromURL:()=>model})}},awt:{Point:struct,Size:struct},beans:{PropertyValue:struct},util:{XModifyListener:{}},view:{XSelectionChangeListener:{}}}
 const messages:any[]=[],port:any={postMessage:(v:any)=>messages.push(v)},zeta={uno:{com:{sun:{star:css}}},Any:function(this:any,_:any,v:any){return v},sameUnoObject:(a:any,b:any)=>a===b,getUnoComponentContext(){},unoObject:(_:any,v:any)=>v,mainPort:port}
 // Any is a wrapper in the native bridge; unwrap in this minimal test model.
 shapes.forEach((shape,i)=>{shape.setPropertyValue=(_:string,n:any)=>{objects[i].fill=typeof n==='object'?n.value:n;listener.modified()}})
 zeta.Any=function(this:any,_:any,v:any){this.value=v} as any
 vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js',import.meta.url),'utf8'),{TextEncoder,Module:{zetajs:{then:(f:any)=>f(zeta)}}})
 let id=0;const send=(command:string,extra:any={})=>{port.onmessage({data:{command,channel:'channel',operationId:`operation_${++id}`,documentId:'d'.repeat(64),version:'a'.repeat(64),...extra}});return messages.at(-1)}
 send('bind');send('open',{kind:'pptx'});send('edit');const captured=send('captureSelection').selection
 const before=structuredClone(captured.presentation),after=structuredClone(before)
 const apply=()=>send('replacePresentation',{selectionToken:captured.token,expectedChangeSequence:0,presentation:{before,after}})
 return {send,before,after,apply,objects,shapes,model,fail:()=>{failSize=true},changeTarget:()=>{selectedIndex=1}}
}
test('typed shape geometry and fill mutate only the captured shape and preserve text',()=>{
 for(const kind of ['geometry','fill']){const h=fixture();expect(nativePresentationSelectionSchema.safeParse(h.before).success).toBe(true);if(kind==='geometry')Object.assign(h.after.shapes[0],{x100thMm:120,width100thMm:110});else h.after.shapes[0].fillRGB='#abcdef';expect(h.apply()).toMatchObject({ok:true});expect(h.objects[0].text).toBe('text 0');expect(h.objects[1]).toMatchObject({x:300,fill:0x123456});expect(h.send('export').ok).toBe(true)}
})
test('no-op, mixed geometry/fill and changes outside captured target fail before mutation',()=>{
 for(const kind of ['noop','mixed','other','outOfBounds']){const h=fixture();if(kind==='mixed')Object.assign(h.after.shapes[0],{x100thMm:120,fillRGB:'#abcdef'});if(kind==='other')h.after.shapes[1].fillRGB='#abcdef';if(kind==='outOfBounds')h.after.shapes[0].x100thMm=1000;expect(nativePresentationReviewSchema.safeParse({before:h.before,after:h.after}).success).toBe(false);expect(h.apply().ok).toBe(false);expect(h.objects[0].x).toBe(100);expect(h.objects[0].fill).toBe(0x123456)}
})
test('silent shape changes, order swaps and partial writes cannot be exported',()=>{
 const stale=fixture();stale.after.shapes[0].x100thMm=120;stale.objects[1].fill=1;expect(stale.apply()).toMatchObject({ok:false,error:'stale-selection'});expect(stale.objects[0].x).toBe(100)
 const swap=fixture();swap.after.shapes[0].x100thMm=120;swap.shapes.reverse();expect(swap.apply().ok).toBe(false)
 const failed=fixture();failed.after.shapes[0].x100thMm=120;failed.fail();expect(failed.apply()).toMatchObject({ok:false,error:'typed-mutation-failed'});expect(failed.objects[0].x).toBe(120);expect(failed.send('export')).toMatchObject({ok:false,error:'typed-mutation-failed'});expect(failed.model.storeToURL).not.toHaveBeenCalled()
})
test('preload protocol rejects arbitrary native fields, unknown properties and mixed review',()=>{
 const h=fixture();h.after.shapes[0].fillRGB='#abcdef';const request={command:'replacePresentation',operationId:'operation_1',channel:'channel',documentId:'d'.repeat(64),version:'a'.repeat(64),selectionToken:'selection_1',expectedChangeSequence:0,presentation:{before:h.before,after:h.after}}
 expect(isOfficeRequest(request)).toBe(true);expect(isOfficeRequest({...request,uno:'Shell'})).toBe(false);h.after.shapes[0].x100thMm=120;expect(isOfficeRequest(request)).toBe(false)
})
