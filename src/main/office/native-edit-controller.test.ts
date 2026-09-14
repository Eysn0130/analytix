import { createHash } from 'node:crypto'
import { expect, test } from 'vitest'
import { createNativeOfficeController, type NativeOfficeEngineRequest } from './native-office-controller'
import type { PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'
const hash = (value: Uint8Array) => createHash('sha256').update(value).digest('hex')
const original = new Uint8Array([80,75,1]), edited = new Uint8Array([80,75,2])
const objectId = 'a'.repeat(64), sessionId = 'e'.repeat(48), revision = hash(original)
const open = {action:'open',workspace:'/workspace',path:'a.docx',bounds:{x:0,y:0,width:800,height:600}}
function setup() {
  let current = {documentId:objectId,version:revision,kind:'docx' as const,changeSequence:0,acknowledgedSequence:0,dirty:false}
  let onEvent:(event:unknown)=>void = ()=>{}, opened = '', saved = revision, commitCalls=0, statusCalls=0
  let receipt:any, deferCommit:((value:any)=>void)|undefined
  let delay=false, conflict=false, hidden=0, destroyed=0, revoked=0, loseCommit=false, failReload=false, rejectCommit=false
  const scopeId='d'.repeat(48), proposalId='f'.repeat(48)
  let scope:any, loseDecision=false, acceptedOperation:string|undefined, replacementText='proposed'
  let openGate:Promise<void>|undefined, releaseOpen:(()=>void)|undefined
  const pkg:PluginPackageView={packageId:'analytix-documents',packageVersion:'1.0.0',displayName:'Documents',origin:'development-source',publishable:false,materialized:true,generationId:'b'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'c'.repeat(64),available:true,operations:['open-object','close-object','commit-object','object-status','capture-selection','proposal-read','proposal-accept','proposal-reject','selection-revoke']}
  const selection=()=>({documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'text',scope:'session-text-range-at-version-and-change-sequence',text:'selected',token:'selection_123',capture:{capturedCharacters:8,totalCharacters:8,truncated:false,unit:'utf-16',complete:true}})
  const requests:NativeOfficeEngineRequest[]=[]
  const controller=createNativeOfficeController({
    async packageHost(r):Promise<any>{
      if(r.action==='list')return {ok:true,packages:[pkg]}
      if(r.action!=='invoke')throw Error('not authorized')
      if(r.operation==='open-object'){await openGate;return {ok:true,output:{ok:true,document:{objectId,sessionId,path:'/workspace/a.docx',revision:saved,content:Buffer.from(saved===revision?original:edited).toString('base64')}}}}
      if(r.operation==='capture-selection'){scope={...r.input,scopeId,parts:[{kind:'literal',text:'selected'}]};delete scope.text;return {ok:true,output:{ok:true,scope}}}
      if(r.operation==='proposal-read')return {ok:true,output:{ok:true,proposals:[{proposalId,status:'proposed',parts:[{kind:'literal',text:'proposed'}]}]}}
      if(r.operation==='selection-revoke'){revoked++;return {ok:true,output:{ok:true,revoked:true}}}
      if(r.operation==='proposal-accept'){if(acceptedOperation)expect(r.input.operationId).toBe(acceptedOperation);acceptedOperation=r.input.operationId as string;if(loseDecision){loseDecision=false;throw Error('transport lost after Core accepted')}return {ok:true,output:{ok:true,replacement:{proposalId,operationId:r.input.operationId,text:replacementText,selectionToken:scope.selectionToken,changeSequence:scope.changeSequence,baseRevision:scope.baseRevision}}}}
      if(r.operation==='close-object')return {ok:true,output:{ok:true,closed:true}}
      if(r.operation==='commit-object'){
        commitCalls++;const input=r.input as any
        if(rejectCommit){rejectCommit=false;return {ok:true,output:{ok:false,code:'not_text',message:'unsupported'}}}
        const content = new Uint8Array(Buffer.from(input.content.data, 'base64'))
        expect(input.content).toEqual({encoding:'base64',kind:'docx',data:Buffer.from(content).toString('base64'),sha256:hash(content),byteLength:3})
        expect(input.baseRevision).toBe(saved)
        receipt={operationId:input.operationId,revision:conflict?'':hash(content),status:conflict?'conflict':'committed',savedAt:new Date().toISOString()}
        if(!conflict)saved=hash(content)
        if(loseCommit){loseCommit=false;throw Error('reply lost after commit')}
        if(delay)return await new Promise(resolve=>{deferCommit=resolve})
        return {ok:true,output:{ok:true,receipt}}
      }
      if(r.operation==='object-status'){statusCalls++;return {ok:true,output:{ok:true,receipt}}}
    },
    createSurface(listener){onEvent=listener;return {attach(){},hide(){hidden++},destroy(){destroyed++},async request(r){
      requests.push(r)
      const base={type:'result',channel:'surface',command:r.command,operationId:r.operationId,documentId:r.documentId,version:r.version,ok:true}
      if(r.command==='open'){if(failReload){failReload=false;throw Error('reload failed')}opened=r.operationId;current={...current,version:r.version,changeSequence:0,acknowledgedSequence:0,dirty:false}}
      if(r.command==='close')return base
      if(r.command==='replace')current={...current,changeSequence:current.changeSequence+1,dirty:true}
      if(r.command==='export')return {...base,state:{...current},bytes:edited,exportedSequence:current.changeSequence}
      if(r.command==='ack'&&r.status==='committed')current={...current,version:r.persistedVersion!,acknowledgedSequence:r.exportedSequence!,dirty:current.changeSequence!==r.exportedSequence}
      return {...base,state:{...current},selection:selection()}
    }}},onChange(){}})
  const change=()=>{current={...current,changeSequence:current.changeSequence+1,dirty:true};onEvent({type:'changed',operationId:opened,documentId:objectId,version:current.version,state:{...current}})}
  const target=()=>({objectId,revision:current.version,expectedChangeSequence:current.changeSequence})
  return {controller,target,change,requests,pkg,scopeId,proposalId,getRevoked:()=>revoked,getSaved:()=>saved,externalEdit:()=>{saved=hash(edited)},rejectCommit:()=>{rejectCommit=true},loseCommit:()=>{loseCommit=true},failReload:()=>{failReload=true},loseDecision:()=>{loseDecision=true},oversize:()=>{replacementText='x'.repeat(4097)},holdOpen:()=>{openGate=new Promise(resolve=>{releaseOpen=resolve})},releaseOpen:()=>releaseOpen?.(),delay:()=>{delay=true},conflict:()=>{conflict=true},complete:()=>deferCommit?.({ok:true,output:{ok:true,receipt}}),getCommitCalls:()=>commitCalls,getStatusCalls:()=>statusCalls,getHidden:()=>hidden,getDestroyed:()=>destroyed}
}
test('native annotation preparation is explicit; persisted receipt advances revision, close unknown object never closes active',async()=>{
  const h=setup();expect((await h.controller.request(open)).ok).toBe(true)
  expect((await h.controller.request({action:'annotate',...h.target()})).ok).toBe(true);h.change()
  expect(await h.controller.request({action:'close'})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:true,view:{revision:hash(edited),dirty:false}})
  expect(h.getCommitCalls()).toBe(1)
  expect((await h.controller.request({action:'close',objectId:'f'.repeat(64)})).view?.objectId).toBe(objectId)
  expect(h.getDestroyed()).toBe(0)
  expect(await h.controller.request({action:'close'})).toEqual({ok:true,view:null})
})
test('a later engine change remains dirty after an older receipt and stale annotation targets are rejected',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()});h.change();h.delay()
  const frozen=h.target(), saving=h.controller.request({action:'save',...frozen})
  while(h.getCommitCalls()===0)await new Promise(resolve=>setTimeout(resolve,1))
  h.change();h.complete()
  expect(await saving).toMatchObject({ok:true,view:{changeSequence:2,acknowledgedSequence:1,dirty:true}})
  expect(await h.controller.request({action:'annotate',...frozen})).toMatchObject({ok:false,error:'stale_selection'})
})
test('conflict and disabled plugin preserve native dirty state and never fabricate Saved',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()});h.change();h.conflict()
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'conflict',view:{dirty:true}})
  h.pkg.available=false;h.pkg.desiredState='disabled'
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'package_unavailable',view:{dirty:true}})
  expect(h.getCommitCalls()).toBe(1);expect(h.getDestroyed()).toBe(0)
})

test('trusted native proposal applies only the bound selection and revokes its scope after native change',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()})
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:'forged_token',threadId:'thread-native',editable:true})).toMatchObject({ok:false,error:'stale_selection'})
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId:'thread-native',editable:true})).toMatchObject({ok:true,view:{scope:{scopeId:h.scopeId}}})
  const result=await h.controller.request({action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId})
  expect(result).toMatchObject({ok:true,view:{dirty:false,canUndo:true,revision:hash(edited),changeSequence:1,appliedProposals:[h.proposalId]}})
  await h.controller.request({action:'status'})
  expect(h.getRevoked()).toBe(1)
  expect(h.requests.filter(r=>r.command==='replace')).toEqual([expect.objectContaining({text:'proposed',selectionToken:'selection_123',expectedChangeSequence:0,valueType:'text'})])
  await h.controller.request({action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId})
  expect(h.requests.filter(r=>r.command==='replace')).toHaveLength(1)
})
test('local changes revoke captured scopes before another acceptance and status reads do not hide the surface',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()})
  await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId:'thread-native',editable:true})
  h.change();await h.controller.request({action:'status'})
  expect(h.getRevoked()).toBe(1)
  expect(await h.controller.request({action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId})).toMatchObject({ok:false,error:'stale_selection'})
  expect(h.requests.some(r=>r.command==='replace')).toBe(false)
})

test('a hidden or superseded slow open cannot attach a late native surface',async()=>{
  const h=setup();h.holdOpen();const pending=h.controller.request(open)
  await new Promise(resolve=>setTimeout(resolve,0))
  await h.controller.request({action:'hide'});h.releaseOpen()
  expect(await pending).toMatchObject({ok:false,view:null})
  expect(h.requests).toHaveLength(0)
})

test('a lost Core approval reply reuses its decision operation and applies exactly once',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()})
  await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId:'thread-native',editable:true})
  h.loseDecision()
  const accept={action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId}
  expect(await h.controller.request(accept)).toMatchObject({ok:false,error:'unavailable'})
  expect(await h.controller.request(accept)).toMatchObject({ok:true,view:{appliedProposals:[h.proposalId]}})
  expect(h.requests.filter(r=>r.command==='replace')).toHaveLength(1)
})
test('an oversized invalid proposal reply cannot destroy the workcopy',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()})
  await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId:'thread-native',editable:true})
  h.oversize()
  expect(await h.controller.request({action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId})).toMatchObject({ok:false,error:'invalid_response'})
  expect(h.getDestroyed()).toBe(0)
  expect((await h.controller.request({action:'captureSelection'})).ok).toBe(true)
})


test('renderer manual edit commands are rejected before reaching the native engine',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()})
  const count=h.requests.length
  for(const action of ['edit','format','replace','undo','redo']) {
    expect(await h.controller.request({action,...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  }
  expect(h.requests).toHaveLength(count)
})

async function applyProposal(h: ReturnType<typeof setup>) {
  await h.controller.request(open)
  await h.controller.request({action:'annotate',...h.target()})
  await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId:'thread-native',editable:true})
  return h.controller.request({action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId})
}
test('accept automatically persists and undo restores exactly the previous file once',async()=>{
  const h=setup();expect(await applyProposal(h)).toMatchObject({ok:true,view:{canUndo:true,dirty:false}})
  expect(h.getCommitCalls()).toBe(1);expect(h.getSaved()).toBe(hash(edited))
  expect(h.controller.getView()?.scope).toBeUndefined()
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:true,view:{canUndo:false,dirty:false,revision,editing:false}})
  expect(h.getSaved()).toBe(revision);expect(h.getCommitCalls()).toBe(2)
  expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:original,version:revision})
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(h.getCommitCalls()).toBe(2)
})
test('undo conflict preserves the current canvas and never reloads a rejected rollback',async()=>{
  const h=setup();await applyProposal(h);h.conflict()
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:false,error:'conflict',view:{revision:hash(edited)}})
  expect(h.getSaved()).toBe(hash(edited));expect(h.requests.filter(r=>r.command==='open')).toHaveLength(1)
})
test('lost automatic commit reply is reconciled without applying or committing twice',async()=>{
  const h=setup();h.loseCommit()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'unknown',view:{dirty:true,saving:true,canUndo:false}})
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{dirty:false,saving:false,canUndo:true}})
  expect(h.getCommitCalls()).toBe(1);expect(h.requests.filter(r=>r.command==='replace')).toHaveLength(1)
})
test('lost undo receipt queries the original operation and then reloads, without another write',async()=>{
  const h=setup();await applyProposal(h);h.loseCommit()
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(await h.controller.request({action:'close'})).toMatchObject({ok:false,error:'unknown'})
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{revision,canUndo:false,saving:false}})
  expect(h.getCommitCalls()).toBe(2);expect(h.getStatusCalls()).toBe(1)
})
test('a failed undo reload remains recoverable and never repeats the committed rollback',async()=>{
  const h=setup();await applyProposal(h);h.failReload()
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{revision,canUndo:false,saving:false}})
  expect(h.getCommitCalls()).toBe(2);expect(h.getStatusCalls()).toBe(0)
})

test('definitively rejected undo releases pending state and can be retried or closed',async()=>{
  const h=setup();await applyProposal(h);h.rejectCommit()
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:false,error:'save_failed',view:{saving:false,canUndo:true}})
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:true,view:{canUndo:false,revision}})
  expect(h.getStatusCalls()).toBe(0)
  expect(await h.controller.request({action:'close'})).toMatchObject({ok:true,view:null})
})
test('undo reload rechecks current file rather than treating a historical receipt as current state',async()=>{
  const h=setup();await applyProposal(h);h.failReload()
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:false,error:'unknown'})
  h.externalEdit()
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'conflict',view:{saving:false,status:'error',canUndo:false}})
  expect(h.getSaved()).toBe(hash(edited));expect(h.getCommitCalls()).toBe(2)
  expect(await h.controller.request(open)).toMatchObject({ok:true,view:{revision:hash(edited),status:'ready'}})
})
