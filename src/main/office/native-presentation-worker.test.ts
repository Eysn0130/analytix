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

async function playbackFixture(options: {unsupported?: boolean; pending?: boolean} = {}) {
 const messages: any[] = [], replies = new Map<string,(value:any)=>void>()
 let running = false, paused = false, showIndex = -1, effect = 0, modified = false, modifyListener:any
 const drawPages = Array.from({length:5},(_,index)=>({getName:()=>`slide-${index+1}`,getCount:()=>0,getPropertyValue:()=>1000}))
 let currentPage = drawPages[4]
 const selected = {getCount:()=>0}
 const show = {
  isRunning:()=>running,isPaused:()=>paused,getSlideCount:()=>2,getCurrentSlideIndex:()=>showIndex,
  gotoNextEffect:vi.fn(()=>{effect++}),gotoPreviousEffect:vi.fn(()=>{effect--}),
  gotoNextSlide:vi.fn(()=>{showIndex=1}),gotoPreviousSlide:vi.fn(()=>{showIndex=0}),
  pause:vi.fn(()=>{paused=true}),resume:vi.fn(()=>{paused=false})
 }
 const presentation = {
  isRunning:()=>running,getController:()=>running ? show : null,
  startWithArguments:vi.fn(()=>{running=!options.pending;paused=false}),end:vi.fn(()=>{running=false})
 }
 let zoom=175, zoomType=3
 const controller = {
  getCurrentPage:()=>currentPage,setCurrentPage:vi.fn((page:any)=>{currentPage=page}),
  getSelection:()=>selected,select:vi.fn(()=>true),
  getPropertyValue:(name:string)=>name==='ZoomValue'?zoom:zoomType,
  getPropertySetInfo:()=>({getPropertyByName:()=>({Type:{}})}),
  setPropertyValue:(name:string,value:any)=>{if(name==='ZoomValue')zoom=value.value;else zoomType=value.value},
  getFrame:()=>({getContainerWindow:()=>({}),LayoutManager:{setVisible(){},isVisible:()=>false}}),
  addSelectionChangeListener(){},removeSelectionChangeListener(){}
 }
 const model = {
  isReadonly:()=>true,isModified:()=>modified,setModified:vi.fn(),
  getDrawPages:()=>({getCount:()=>drawPages.length,getByIndex:(index:number)=>drawPages[index]}),
  getCurrentController:()=>controller,getPresentation:options.unsupported?undefined:()=>presentation,
  addModifyListener:(listener:any)=>{modifyListener=listener},removeModifyListener(){},close:vi.fn(()=>{running=false})
 }
 const load=vi.fn((_url:string,_target:string,_flags:number,_args:any[])=>model), struct=function(this:any,value:any){Object.assign(this,value)}
 const css={frame:{Desktop:{create:()=>({loadComponentFromURL:load})}},beans:{PropertyValue:struct},util:{XModifyListener:{}},view:{XSelectionChangeListener:{}}}
 const port:any={postMessage:(value:any)=>{messages.push(value);if(value.type==='result'){replies.get(value.operationId)?.(value);replies.delete(value.operationId)}}}
 const zeta={uno:{com:{sun:{star:css}}},Any:function(this:any,_:any,value:any){this.value=value},
  sameUnoObject:(a:any,b:any)=>a===b,getUnoComponentContext(){},unoObject:(_:any,value:any)=>value,mainPort:port}
 vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js',import.meta.url),'utf8'),
  {TextEncoder,setTimeout,setInterval,clearInterval,Module:{zetajs:{then:(fn:any)=>fn(zeta)}}})
 port.onmessage({data:{command:'bind',channel:'channel'}})
 let operation=0
 const send=(command:string,extra:any={})=>new Promise<any>(resolve=>{
  const operationId=`playback_${++operation}`;replies.set(operationId,resolve)
  port.onmessage({data:{command,channel:'channel',operationId,documentId:'d'.repeat(64),version:'a'.repeat(64),...extra}})
 })
 await send('open',{kind:'pptx'})
 const play=(action:string,generation?:number)=>send('local-view',{action,...(generation===undefined?{}:{playbackGeneration:generation})})
 return {play,send,messages,model,presentation,show,controller,load,drawPages,selected,
  effect:()=>effect,setIndex:(index:number)=>{showIndex=index},finish:()=>{running=false},lateStart:()=>{running=true},modify:()=>{modified=true;modifyListener.modified()},
  close:()=>send('close',{expectedChangeSequence:0,discard:true})}
}

test('real presentation calls retain editor metadata and use the independent custom-show sequence',async()=>{
 vi.useFakeTimers();const h=await playbackFixture()
 try {
  const before=await h.play('play-read')
  expect(before.view).toMatchObject({zoom:175,page:5,pages:5,playback:{generation:0,phase:'idle',slideIndex:null,slideCount:null}})
  const started=await h.play('play-start',0)
  expect(started).toMatchObject({ok:true,view:{zoom:175,page:5,pages:5,playback:{generation:1,phase:'running',slideIndex:-1,slideCount:2}}})
  expect(h.presentation.startWithArguments).toHaveBeenCalledWith([{Name:'IsFullScreen',Value:false},{Name:'UsePen',Value:false}])
  const effect=await h.play('play-next-effect',1)
  expect(h.show.gotoNextEffect).toHaveBeenCalledOnce();expect(h.effect()).toBe(1)
  expect(effect.view.playback.slideIndex).toBe(-1)
  expect(effect.view.playback.sequence).toBeGreaterThan(started.view.playback.sequence)
  expect((await h.play('play-next-slide',1)).view.playback.slideIndex).toBe(1)
  expect(h.show.gotoNextSlide).toHaveBeenCalledOnce()
  expect((await h.play('play-pause',1)).view.playback.phase).toBe('paused')
  expect((await h.play('play-resume',1)).view.playback.phase).toBe('running')
  const ended=await h.play('play-end',1)
  expect(ended.view.playback).toMatchObject({generation:2,phase:'idle',slideIndex:null,slideCount:null})
  expect(h.controller.setCurrentPage).toHaveBeenLastCalledWith(h.drawPages[4])
  expect(h.controller.select).toHaveBeenLastCalledWith(h.selected)
  expect(h.model.isReadonly()).toBe(true);expect(h.model.isModified()).toBe(false);expect(h.model.setModified).not.toHaveBeenCalled()
  expect(h.load.mock.calls[0][3]).toEqual(expect.arrayContaining([{Name:'MacroExecutionMode',Value:0},{Name:'ReadOnly',Value:true},{Name:'LockEditDoc',Value:true},{Name:'LockSave',Value:true}]))
  expect(vi.getTimerCount()).toBe(0)
 } finally {await h.close();vi.clearAllTimers();vi.useRealTimers()}
})

test('only the explicit current-slide entry passes FirstPage and stale generations cannot mutate playback',async()=>{
 vi.useFakeTimers();const h=await playbackFixture()
 try {
  const started=await h.play('play-current',0)
  expect(h.presentation.startWithArguments).toHaveBeenCalledWith([{Name:'IsFullScreen',Value:false},{Name:'UsePen',Value:false},{Name:'FirstPage',Value:'slide-5'}])
  expect(await h.play('play-next-effect',0)).toMatchObject({ok:false,error:'stale-selection'})
  expect(h.show.gotoNextEffect).not.toHaveBeenCalled()
  expect(await h.play('play-start',started.view.playback.generation)).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(await h.send('edit')).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(await h.send('replacePresentation')).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(await h.send('export')).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(await h.play('play-read',1)).toMatchObject({ok:false,error:'invalid-request'})
 } finally {await h.close();vi.clearAllTimers();vi.useRealTimers()}
})

test('missing native presentation capability emits a truthful unsupported view before rejecting start',async()=>{
 vi.useFakeTimers();const h=await playbackFixture({unsupported:true})
 try {
  const result=await h.play('play-start',0)
  expect(result).toMatchObject({ok:false,error:'presentation-unsupported'})
  const index=h.messages.indexOf(result), failure=h.messages.slice(0,index).reverse().find(value=>value.type==='view')
  expect(failure.view).toMatchObject({zoom:175,page:5,pages:5,playback:{generation:1,phase:'unsupported',slideIndex:null,slideCount:null}})
  expect((await h.play('play-read')).view.playback.phase).toBe('unsupported')
  expect(h.model.setModified).not.toHaveBeenCalled();expect(vi.getTimerCount()).toBe(0)
 } finally {await h.close();vi.clearAllTimers();vi.useRealTimers()}
})

test('native timed slide changes and natural termination are sampled, while close retires the timer',async()=>{
 vi.useFakeTimers();const h=await playbackFixture()
 try {
  const start=await h.play('play-start',0)
  h.setIndex(1);await vi.advanceTimersByTimeAsync(250)
  const update=[...h.messages].reverse().find(value=>value.type==='view')
  expect(update.view.playback).toMatchObject({generation:1,phase:'running',slideIndex:1,slideCount:2})
  expect(update.view.playback.sequence).toBeGreaterThan(start.view.playback.sequence)
  h.finish();await vi.advanceTimersByTimeAsync(250)
  expect([...h.messages].reverse().find(value=>value.type==='view').view.playback).toMatchObject({generation:2,phase:'idle'})
  expect(vi.getTimerCount()).toBe(0)
  await h.play('play-start',2);await h.close()
  expect(h.model.close).toHaveBeenCalledOnce();expect(vi.getTimerCount()).toBe(0)
 } finally {vi.clearAllTimers();vi.useRealTimers()}
})

test('a timed-out pending native startup stays blocked even if the callback starts after end returned',async()=>{
 vi.useFakeTimers();const h=await playbackFixture({pending:true})
 try {
  const pending=h.play('play-start',0)
  await vi.advanceTimersByTimeAsync(5000)
  expect(await pending).toMatchObject({ok:false,error:'presentation-start-timeout'})
  h.lateStart()
  expect((await h.play('play-read')).view.playback.phase).toBe('error')
  expect(await h.send('edit')).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(await h.send('local-view',{action:'fit'})).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(await h.play('play-start',1)).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(await h.play('play-end',1)).toMatchObject({ok:false,error:'presentation-start-timeout'})
  expect(h.model.setModified).not.toHaveBeenCalled()
 } finally {await h.close();vi.clearAllTimers();vi.useRealTimers()}
})

test('playback cannot clear a native modification to fabricate a readonly success',async()=>{
 vi.useFakeTimers();const h=await playbackFixture()
 try {
  await h.play('play-start',0);h.modify()
  expect(await h.play('play-read')).toMatchObject({ok:false,error:'presentation-failed'})
  expect([...h.messages].reverse().find(value=>value.type==='view').view).toMatchObject({modified:true,playback:{phase:'error'}})
  expect([...h.messages].reverse().find(value=>value.type==='changed').state).toMatchObject({dirty:true,changeSequence:1,acknowledgedSequence:0})
  expect(h.model.isModified()).toBe(true);expect(h.model.setModified).not.toHaveBeenCalled()
  expect(await h.send('edit')).toMatchObject({ok:false,error:'operation-in-progress'})
  expect(vi.getTimerCount()).toBe(0)
 } finally {await h.close();vi.clearAllTimers();vi.useRealTimers()}
})
