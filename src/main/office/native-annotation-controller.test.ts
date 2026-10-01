import { createHash } from 'node:crypto'
import { expect, test, vi } from 'vitest'
import { createNativeOfficeController } from './native-office-controller'
import type { NativeOfficeAnnotation } from '../../../packages/runtime/src/contracts/native-office-editing'
import type { PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'

const hash=(value:string|Uint8Array)=>createHash('sha256').update(value).digest('hex')
const bytes=new Uint8Array([80,75,1]), revision=hash(bytes), threadId='thread-one'
const objectId=(name='a')=>hash(name), sessionId=(name='a')=>hash(`session-${name}`).slice(0,48)
const open=(name='a',thread=threadId)=>({action:'open',workspace:'/workspace',path:`${name}.docx`,threadId:thread,bounds:{x:0,y:0,width:800,height:600}})
const save=(note:string,name='a',thread=threadId)=>({action:'annotationSave',objectId:objectId(name),threadId:thread,note,sourceRevision:revision})
const operations=['open-object','object-status','close-object','annotation-read','annotation-write','commit-object','object-recovery','undo-change','cancel-change','resume-change','capture-selection','selection-read','selection-revoke','proposal-read','proposal-accept','proposal-reject','model-selection-read','model-selection-propose']
function setup(freezeAnnotationInput?: (frozen:boolean)=>Promise<boolean>){
  const stored=new Map<string,NativeOfficeAnnotation>(), attempts=new Map<string,Record<string,unknown>>()
  const writes:Record<string,unknown>[]=[], events:string[]=[], engine:string[]=[]
  let failed=false, lost=false, destroyed=0, gate:Promise<void>|undefined, release:(()=>void)|undefined
  const key=(id:string,thread:string)=>`${id}:${thread}`
  const empty=(id:string,thread:string):NativeOfficeAnnotation=>({objectId:id,threadId:thread,draftRevision:'',note:'',sourceRevision:'',updatedAt:''})
  const pkg:PluginPackageView={packageId:'analytix-documents',packageVersion:'1',displayName:'Documents',origin:'development-source',publishable:false,materialized:true,generationId:'b'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'c'.repeat(64),available:true,operations}
  const controller=createNativeOfficeController({freezeAnnotationInput,
    async packageHost(request):Promise<any>{
      if(request.action==='list')return {ok:true,packages:[pkg]}
      if(request.action!=='invoke')throw Error('unexpected host action')
      const input=request.input as Record<string,any>
      const name=['a','b','c'].find(name=>sessionId(name)===input.sessionId)
      events.push(`${request.operation}:${name??input.object?.path??''}`)
      if(request.operation==='open-object'){
        const name=String(input.object.path).replace('.docx','')
        return {ok:true,output:{ok:true,document:{objectId:objectId(name),sessionId:sessionId(name),path:`/workspace/${name}.docx`,revision,content:Buffer.from(bytes).toString('base64')}}}
      }
      if(!name)throw Error('unknown fixture session')
      if(request.operation==='close-object')return {ok:true,output:{ok:true,closed:true}}
      if(request.operation==='object-recovery')return {ok:true,output:{ok:true,recovery:{current:null,pending:null}}}
      const id=objectId(name), storageKey=key(id,input.threadId)
      if(request.operation==='annotation-read')return {ok:true,output:{ok:true,annotation:stored.get(storageKey)??empty(id,input.threadId)}}
      if(request.operation==='annotation-write'){
        writes.push(structuredClone(input));await gate
        if(failed)throw Error('annotation storage unavailable')
        const previous=stored.get(storageKey)??empty(id,input.threadId), previousAttempt=attempts.get(storageKey)
        if(previousAttempt && JSON.stringify(previousAttempt)===JSON.stringify(input))return {ok:true,output:{ok:true,annotation:previous}}
        expect(Object.keys(input).sort()).toEqual(['expectedDraftRevision','note','sessionId','sourceRevision','threadId'])
        if(input.expectedDraftRevision!==previous.draftRevision)return {ok:true,output:{ok:false,code:'conflict',message:'draft conflict'}}
        const annotation={objectId:id,threadId:input.threadId,draftRevision:hash(JSON.stringify(input)),note:input.note,sourceRevision:input.sourceRevision,updatedAt:'2026-09-15T00:00:00Z'}
        stored.set(storageKey,annotation);attempts.set(storageKey,structuredClone(input))
        if(lost){lost=false;throw Error('acknowledgement lost after CAS')}
        return {ok:true,output:{ok:true,annotation}}
      }
      throw Error('unimplemented fixture operation')
    },
    createSurface(){return {attach(){},hide(){},destroy(){destroyed++},async request(input){
      engine.push(input.command)
      const base={type:'result',channel:'surface',command:input.command,operationId:input.operationId,documentId:input.documentId,version:input.version,ok:true}
      if(input.command==='close')return base
      return {...base,state:{documentId:input.documentId,version:input.version,kind:'docx',changeSequence:0,acknowledgedSequence:0,dirty:false},selection:{documentId:input.documentId,version:input.version,changeSequence:0,kind:'unavailable',scope:'engine selection interface unavailable in this view'}}
    }}},onChange(){}}
  )
  return {controller,writes,events,engine,getDestroyed:()=>destroyed,fail:(value=true)=>{failed=value},loseAck:()=>{lost=true},hold:()=>{gate=new Promise(resolve=>{release=resolve})},release:()=>{release?.();gate=undefined},read:(name='a',thread=threadId)=>stored.get(key(objectId(name),thread)),seed:(note:string,name='a',thread=threadId)=>stored.set(key(objectId(name),thread),{...empty(objectId(name),thread),note,sourceRevision:'d'.repeat(64),draftRevision:hash(note),updatedAt:'2026-09-14T00:00:00Z'})}
}

test('all 18 finite capabilities admit and disk annotation restores text without selection authority',async()=>{
  const h=setup();expect(operations).toHaveLength(18);h.seed('恢复备注')
  const result=await h.controller.request(open());expect(result,JSON.stringify({result,events:h.events,engine:h.engine})).toMatchObject({ok:true,view:{annotation:{note:'恢复备注',sourceRevision:'d'.repeat(64)}}})
  expect(h.controller.getView()?.selection).toMatchObject({kind:'unavailable'});expect(h.controller.getView()?.selection?.token).toBeUndefined();expect(h.controller.getView()?.scope).toBeUndefined()
  expect(h.controller.getView()?.editing).not.toBe(true);expect(h.controller.hasPendingAnnotations()).toBe(false)
  expect(h.engine).toEqual(['open'])
})
test('Main records desired input synchronously and coalesces later keystrokes behind an exact in-flight CAS',async()=>{
  const h=setup();await h.controller.request(open());h.hold()
  const first=h.controller.request(save('first'))
  expect(h.controller.hasPendingAnnotations()).toBe(true)
  expect(h.controller.getView()?.annotationSaving).toBe(true)
  await expect.poll(()=>h.writes.length).toBe(1)
  const second=h.controller.request(save('second')), third=h.controller.request(save('latest'))
  h.release();expect((await first).ok).toBe(true);await second;await third
  expect(h.writes.map(write=>write.note)).toEqual(['first','latest'])
  expect(h.writes[0].expectedDraftRevision).toBe('')
  expect(h.writes[1].expectedDraftRevision).toBe(hash(JSON.stringify(h.writes[0])))
  expect(h.read()?.note).toBe('latest');expect(h.controller.hasPendingAnnotations()).toBe(false)
})
test('a lost CAS acknowledgement retries the exact attempt before saving newly typed input',async()=>{
  const h=setup();await h.controller.request(open());h.loseAck()
  expect(await h.controller.request(save('first'))).toMatchObject({ok:false,view:{annotationError:true}})
  expect(h.read()?.note).toBe('first');expect(h.controller.hasPendingAnnotations()).toBe(true)
  expect((await h.controller.request(save('second'))).ok).toBe(true)
  expect(h.writes).toHaveLength(3);expect(h.writes[1]).toEqual(h.writes[0])
  expect(h.writes[2]).toMatchObject({note:'second',expectedDraftRevision:hash(JSON.stringify(h.writes[0]))})
  expect(h.read()?.note).toBe('second');expect(h.controller.hasPendingAnnotations()).toBe(false)
})
test.each(['close','switch-thread','evict'] as const)('%s flushes annotations first, preserves failures, then succeeds on retry',async(boundary)=>{
  const h=setup();await h.controller.request(open());h.fail()
  expect((await h.controller.request(save('must survive'))).ok).toBe(false)
  if(boundary==='evict')expect((await h.controller.request(open('b'))).ok).toBe(true)
  const request=boundary==='close'?{action:'close',objectId:objectId()}:boundary==='switch-thread'?open('a','thread-two'):open('c')
  expect((await h.controller.request(request)).ok).toBe(false)
  expect(h.controller.hasPendingAnnotations()).toBe(true)
  expect(h.events).not.toContain('close-object:a');expect(h.getDestroyed()).toBe(0)
  h.fail(false);const start=h.events.length
  expect((await h.controller.request(request)).ok).toBe(true)
  expect(h.read()?.note).toBe('must survive');expect(h.controller.hasPendingAnnotations()).toBe(false)
  const tail=h.events.slice(start)
  expect(tail.indexOf('annotation-write:a')).toBeGreaterThanOrEqual(0)
  const next=boundary==='switch-thread'?'annotation-read:a':'close-object:a'
  expect(tail.indexOf(next)).toBeGreaterThan(tail.indexOf('annotation-write:a'))
  if(boundary==='switch-thread')expect(h.controller.getView()?.annotation).toMatchObject({threadId:'thread-two',note:''})
})
test('explicit flush/retry retains note failures and never creates native edit authority',async()=>{
  const h=setup();await h.controller.request(open());h.fail()
  await h.controller.request(save('pending'))
  expect((await h.controller.flushAnnotations()).ok).toBe(false);expect(h.controller.hasPendingAnnotations()).toBe(true)
  h.fail(false)
  expect((await h.controller.request({action:'annotationRetry',objectId:objectId(),threadId})).ok).toBe(true)
  expect(h.read()?.note).toBe('pending');expect(h.engine).toEqual(['open'])
  expect(await h.controller.request({...save('forged'),expectedDraftRevision:'a'.repeat(64)})).toMatchObject({ok:false,error:'invalid_request'})
  expect((await h.controller.request(save('foreign','a','thread-two'))).ok).toBe(false)
  expect(h.read()?.note).toBe('pending')
})

test('an external draft CAS conflict preserves the exact local attempt and never overwrites the remote note',async()=>{
  const h=setup();expect((await h.controller.request(open())).ok).toBe(true)
  h.seed('external note')
  expect(await h.controller.request(save('local desired'))).toMatchObject({ok:false,error:'conflict',view:{annotationError:true}})
  expect(await h.controller.request({action:'annotationRetry',objectId:objectId(),threadId})).toMatchObject({ok:false,error:'conflict'})
  expect(h.writes).toHaveLength(2);expect(h.writes[1]).toEqual(h.writes[0])
  expect(h.writes[0]).toMatchObject({expectedDraftRevision:'',note:'local desired'})
  expect(h.read()?.note).toBe('external note');expect(h.controller.hasPendingAnnotations()).toBe(true)
  expect(h.engine).toEqual(['open'])
})
test('a closed and reopened object restores Core-persisted notes separately for each thread',async()=>{
  const h=setup();expect((await h.controller.request(open())).ok).toBe(true)
  expect((await h.controller.request(save('thread one durable'))).ok).toBe(true)
  expect((await h.controller.request({action:'close',objectId:objectId()})).ok).toBe(true)
  expect(await h.controller.request(open('a','thread-two'))).toMatchObject({ok:true,view:{annotation:{threadId:'thread-two',note:''}}})
  expect((await h.controller.request(save('thread two durable','a','thread-two'))).ok).toBe(true)
  expect((await h.controller.request({action:'close',objectId:objectId()})).ok).toBe(true)
  expect(await h.controller.request(open())).toMatchObject({ok:true,view:{annotation:{threadId,note:'thread one durable'},selection:{kind:'unavailable'}}})
  expect(h.controller.getView()?.selection?.token).toBeUndefined();expect(h.controller.getView()?.scope).toBeUndefined()
  expect(h.read('a','thread-two')?.note).toBe('thread two durable')
  expect(h.writes).toHaveLength(2);expect(h.controller.hasPendingAnnotations()).toBe(false)
})


test('close includes the last input received before freeze ACK, then rejects inputs until flush and close finish',async()=>{
  let acknowledge!:(ok:boolean)=>void
  const freeze=vi.fn((frozen:boolean)=>frozen?new Promise<boolean>(resolve=>{acknowledge=resolve}):Promise.resolve(true))
  const h=setup(freeze);expect((await h.controller.request(open())).ok).toBe(true)
  const closing=h.controller.request({action:'close',objectId:objectId()})
  await expect.poll(()=>freeze.mock.calls.length).toBe(1)
  expect(h.events).not.toContain('close-object:a')
  h.hold();const lastInput=h.controller.request(save('last key before ACK'))
  expect(h.controller.hasPendingAnnotations()).toBe(true);expect(h.writes).toHaveLength(0)
  acknowledge(true);await expect.poll(()=>h.writes.length).toBe(1)
  expect(await h.controller.request(save('after ACK forbidden'))).toMatchObject({ok:false,error:'unavailable'})
  expect(h.events).not.toContain('close-object:a')
  h.release();expect((await closing).ok).toBe(true);await lastInput
  expect(h.read()?.note).toBe('last key before ACK');expect(h.writes).toHaveLength(1)
  expect(h.events.indexOf('annotation-write:a')).toBeLessThan(h.events.indexOf('close-object:a'))
  expect(freeze.mock.calls).toEqual([[true],[false]])
  expect(h.controller.hasPendingAnnotations()).toBe(false)
})
test('failed freeze ACK refuses close and thaws input without discarding the document',async()=>{
  const freeze=vi.fn(async(frozen:boolean)=>!frozen),h=setup(freeze)
  expect((await h.controller.request(open())).ok).toBe(true)
  expect(await h.controller.request({action:'close',objectId:objectId()})).toMatchObject({ok:false,error:'unavailable'})
  expect(freeze.mock.calls).toEqual([[true],[false]])
  expect(h.events).not.toContain('close-object:a');expect(h.getDestroyed()).toBe(0)
  expect((await h.controller.request(save('still editable note'))).ok).toBe(true)
  expect(h.read()?.note).toBe('still editable note')
})

test('concurrent close and quit hold input frozen until both owners release it',async()=>{
  let acknowledge!:(ok:boolean)=>void
  const freeze=vi.fn((frozen:boolean)=>frozen?new Promise<boolean>(resolve=>{acknowledge=resolve}):Promise.resolve(true))
  const h=setup(freeze);await h.controller.request(open());await h.controller.request(open('b'))
  const closing=h.controller.request({action:'close',objectId:objectId()})
  await expect.poll(()=>freeze.mock.calls.length).toBe(1)
  const quitting=h.controller.freezeAnnotationInput(true)
  acknowledge(true);await quitting;expect((await closing).ok).toBe(true)
  expect(freeze.mock.calls).toEqual([[true]])
  expect(await h.controller.request(save('blocked by quit','b'))).toMatchObject({ok:false,error:'unavailable'})
  await h.controller.freezeAnnotationInput(false)
  expect(freeze.mock.calls).toEqual([[true],[false]])
  expect((await h.controller.request(save('allowed after all release','b'))).ok).toBe(true)
})
test.each(['lost','throw'] as const)('a %s thaw ACK forces a fresh freeze handshake before the next quit',async(mode)=>{
  let failThaw=true, acknowledgeAgain!:(ok:boolean)=>void
  const freeze=vi.fn(async(frozen:boolean)=>{
    if(!frozen&&failThaw){failThaw=false;if(mode==='throw')throw Error('ACK transport lost');return false}
    if(frozen&&!failThaw)return await new Promise<boolean>(resolve=>{acknowledgeAgain=resolve})
    return true
  })
  const h=setup(freeze);await h.controller.request(open());await h.controller.request(open('b'))
  expect(await h.controller.request({action:'close',objectId:objectId()})).toMatchObject({ok:false,error:'unavailable'})
  expect(freeze.mock.calls).toEqual([[true],[false]])
  // A lost thaw ACK must not strand new input in Renderer-only memory.
  expect((await h.controller.request(save('note while thaw ACK is unknown','b'))).ok).toBe(true)
  expect(h.read('b')?.note).toBe('note while thaw ACK is unknown')
  let confirmed=false
  const freezing=h.controller.freezeAnnotationInput(true).then(()=>{confirmed=true})
  await expect.poll(()=>freeze.mock.calls.length).toBe(3)
  expect(freeze.mock.calls).toEqual([[true],[false],[true]]);expect(confirmed).toBe(false)
  acknowledgeAgain(true);await freezing
  expect(await h.controller.request(save('after confirmed freeze','b'))).toMatchObject({ok:false,error:'unavailable'})
  expect(h.read('b')?.note).toBe('note while thaw ACK is unknown')
  await h.controller.freezeAnnotationInput(false)
  expect(freeze.mock.calls).toEqual([[true],[false],[true],[false]])
  expect((await h.controller.request(save('after confirmed thaw','b'))).ok).toBe(true)
})
