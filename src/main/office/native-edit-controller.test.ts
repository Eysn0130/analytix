import { createHash } from 'node:crypto'
import { expect, test } from 'vitest'
import { createNativeOfficeController, type NativeOfficeEngineRequest } from './native-office-controller'
import type { PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'
const hash = (value: Uint8Array) => createHash('sha256').update(value).digest('hex')
const original = new Uint8Array([80,75,1]), edited = new Uint8Array([80,75,2])
const objectId = 'a'.repeat(64), sessionId = 'e'.repeat(48), revision = hash(original)
const threadId = 'thread-native', changeId = '9'.repeat(64), saveOperationId = 'save_core_0001', undoOperationId = 'undo_core_0001'
const open = {action:'open',threadId,workspace:'/workspace',path:'a.docx',bounds:{x:0,y:0,width:800,height:600}}
function setup() {
  let current = {documentId:objectId,version:revision,kind:'docx' as const,changeSequence:0,acknowledgedSequence:0,dirty:false}
  let onEvent:(event:unknown)=>void = ()=>{}, opened = '', saved = revision, commitCalls=0, undoCalls=0, cancelCalls=0, statusCalls=0
  let receipt:any, deferCommit:((value:any)=>void)|undefined
  let delay=false, conflict=false, hidden=0, destroyed=0, revoked=0, loseCommit=false, failReload=false, rejectCommit=false
  const scopeId='d'.repeat(48), proposalId='f'.repeat(48)
  let persisted:any, foreignRecovery=false, corruptOpen=false, corruptReceipt=false
  const receipts = new Map<string, any>()
  let scope:any, loseDecision=false, acceptedOperation:string|undefined, replacementText='proposed'
  let openGate:Promise<void>|undefined, releaseOpen:(()=>void)|undefined
  const pkg:PluginPackageView={packageId:'analytix-documents',packageVersion:'1.0.0',displayName:'Documents',origin:'development-source',publishable:false,materialized:true,generationId:'b'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'c'.repeat(64),available:true,operations:['open-object','close-object','commit-object','object-status','capture-selection','proposal-read','proposal-accept','proposal-reject','selection-revoke','object-recovery','undo-change','cancel-change']}
  const selection=()=>({documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'text',scope:'session-text-range-at-version-and-change-sequence',text:'selected',token:'selection_123',capture:{capturedCharacters:8,totalCharacters:8,truncated:false,unit:'utf-16',complete:true}})
  const requests:NativeOfficeEngineRequest[]=[]
  const createController=()=>createNativeOfficeController({
    async packageHost(r):Promise<any>{
      if(r.action==='list')return {ok:true,packages:[pkg]}
      if(r.action!=='invoke')throw Error('not authorized')
      if(r.operation==='open-object'){await openGate;return {ok:true,output:{ok:true,document:{objectId,sessionId,path:'/workspace/a.docx',revision:saved,content:Buffer.from(corruptOpen?edited:saved===revision?original:edited).toString('base64')}}}}
      if(r.operation==='capture-selection'){scope={...r.input,scopeId,parts:[{kind:'literal',text:'selected'}]};delete scope.text;return {ok:true,output:{ok:true,scope}}}
      if(r.operation==='proposal-read')return {ok:true,output:{ok:true,proposals:[{proposalId,status:'proposed',parts:[{kind:'literal',text:'proposed'}]}],localReviews:[{proposalId,beforeText:'selected raw field',afterText:'proposed raw field'}]}}
      if(r.operation==='object-recovery'){
        const owned=persisted && (persisted.threadId===r.input.threadId || foreignRecovery) ? structuredClone(persisted) : null
        return {ok:true,output:{ok:true,recovery:{current:owned?.revision?owned:null,pending:owned&&!owned.revision?owned:null}}}
      }
      if(r.operation==='cancel-change'){
        cancelCalls++
        expect(r.input).toEqual({sessionId,threadId,changeId,baseRevision:saved})
        expect(persisted.canCancel).toBe(true);expect(persisted.revision).toBe('')
        persisted=undefined
        return {ok:true,output:{ok:true,recovery:{current:null,pending:null}}}
      }
      if(r.operation==='selection-revoke'){revoked++;return {ok:true,output:{ok:true,revoked:true}}}
      if(r.operation==='proposal-accept'){
        if(acceptedOperation)expect(r.input.operationId).toBe(acceptedOperation)
        acceptedOperation=r.input.operationId as string
        persisted ??= {changeId,threadId:scope.threadId,proposalId,baseRevision:scope.baseRevision,revision:'',status:'prepared',beforeText:'selected raw field',afterText:'proposed raw field',saveOperationId,undoOperationId,canUndo:false,canCancel:true,canRetryUndo:false,createdAt:'2026-09-15T00:00:00Z',savedAt:''}
        if(loseDecision){loseDecision=false;throw Error('transport lost after Core accepted')}
        return {ok:true,output:{ok:true,replacement:{proposalId,changeId,saveOperationId,operationId:r.input.operationId,text:replacementText,selectionToken:scope.selectionToken,changeSequence:scope.changeSequence,baseRevision:scope.baseRevision}}}
      }
      if(r.operation==='close-object')return {ok:true,output:{ok:true,closed:true}}
      if(r.operation==='commit-object'){
        commitCalls++;const input=r.input as any
        expect(input).toMatchObject({changeId:persisted.changeId,threadId:persisted.threadId,operationId:persisted.saveOperationId})
        if(rejectCommit){rejectCommit=false;return {ok:true,output:{ok:false,code:'not_text',message:'unsupported'}}}
        const content = new Uint8Array(Buffer.from(input.content.data, 'base64'))
        expect(input.content).toEqual({encoding:'base64',kind:'docx',data:Buffer.from(content).toString('base64'),sha256:hash(content),byteLength:3})
        expect(input.baseRevision).toBe(saved)
        receipt={operationId:input.operationId,revision:conflict?'':corruptReceipt?revision:hash(content),status:conflict?'conflict':'committed',savedAt:new Date().toISOString()}
        if(!conflict)saved=hash(content)
        persisted={...persisted,status:conflict?'conflict':'committed',revision:conflict?'':hash(content),savedAt:receipt.savedAt,canUndo:!conflict,canCancel:false}
        receipts.set(input.operationId,receipt)
        if(loseCommit){loseCommit=false;throw Error('reply lost after commit')}
        if(delay)return await new Promise(resolve=>{deferCommit=resolve})
        return {ok:true,output:{ok:true,receipt}}
      }
      if(r.operation==='undo-change'){
        undoCalls++
        expect(r.input).toEqual({sessionId,threadId,changeId,baseRevision:hash(edited)})
        if(rejectCommit){rejectCommit=false;return {ok:true,output:{ok:false,code:'not_text',message:'unsupported'}}}
        receipt={operationId:undoOperationId,revision:conflict?'':revision,status:conflict?'conflict':'committed',savedAt:'2026-09-15T00:01:00Z'}
        receipts.set(undoOperationId,receipt)
        if(!conflict){saved=revision;persisted={...persisted,status:'undone',canUndo:false,canRetryUndo:false}}
        if(loseCommit){loseCommit=false;throw Error('reply lost after undo')}
        return {ok:true,output:{ok:true,receipt}}
      }
      if(r.operation==='object-status'){statusCalls++;return {ok:true,output:{ok:true,receipt:receipts.get(r.input.operationId as string)}}}
      throw Error('unexpected Core operation')
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
  let controller=createController()
  const change=()=>{current={...current,changeSequence:current.changeSequence+1,dirty:true};onEvent({type:'changed',operationId:opened,documentId:objectId,version:current.version,state:{...current}})}
  const target=()=>({objectId,revision:current.version,expectedChangeSequence:current.changeSequence})
  return {get controller(){return controller},restart:()=>{controller=createController()},corruptOpen:()=>{corruptOpen=true},corruptReceipt:()=>{corruptReceipt=true},foreignRecovery:()=>{foreignRecovery=true},getUndoCalls:()=>undoCalls,getCancelCalls:()=>cancelCalls,markUndoStarted:()=>{persisted={...persisted,status:'unknown',canUndo:false,canRetryUndo:true}},markUncertain:()=>{persisted={...persisted,status:'unknown',canCancel:false}},target,change,requests,pkg,scopeId,proposalId,getRevoked:()=>revoked,getSaved:()=>saved,externalEdit:()=>{saved=hash(edited)},rejectCommit:()=>{rejectCommit=true},loseCommit:()=>{loseCommit=true},failReload:()=>{failReload=true},loseDecision:()=>{loseDecision=true},oversize:()=>{replacementText='x'.repeat(4097)},holdOpen:()=>{openGate=new Promise(resolve=>{releaseOpen=resolve})},releaseOpen:()=>releaseOpen?.(),delay:()=>{delay=true},conflict:()=>{conflict=true},complete:()=>deferCommit?.({ok:true,output:{ok:true,receipt}}),getCommitCalls:()=>commitCalls,getStatusCalls:()=>statusCalls,getHidden:()=>hidden,getDestroyed:()=>destroyed}
}
test('native annotation preparation is explicit; persisted receipt advances revision, close unknown object never closes active',async()=>{
  const h=setup();expect((await h.controller.request(open)).ok).toBe(true)
  expect((await applyProposal(h)).ok).toBe(true)
  expect(h.controller.getView()).toMatchObject({revision:hash(edited),dirty:false})
  expect(h.getCommitCalls()).toBe(1)
  expect((await h.controller.request({action:'close',objectId:'f'.repeat(64)})).view?.objectId).toBe(objectId)
  expect(h.getDestroyed()).toBe(0)
  expect(await h.controller.request({action:'close'})).toEqual({ok:true,view:null})
})
test('a later engine change remains dirty after an older receipt and stale annotation targets are rejected',async()=>{
  const h=setup();await h.controller.request(open);h.delay()
  const frozen=h.target(), saving=applyProposal(h)
  await expect.poll(()=>h.getCommitCalls()).toBe(1)
  h.change();h.complete()
  expect(await saving).toMatchObject({ok:true,view:{changeSequence:2,acknowledgedSequence:1,dirty:true}})
  expect(await h.controller.request({action:'annotate',...frozen})).toMatchObject({ok:false,error:'stale_selection'})
})
test('conflict and disabled plugin preserve native dirty state and never fabricate Saved',async()=>{
  const h=setup();await h.controller.request(open);h.conflict()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'conflict',view:{dirty:true}})
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
  expect(await h.controller.request({action:'undoChange',...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(h.getUndoCalls()).toBe(0)
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:true,view:{canUndo:false,dirty:false,revision,editing:false}})
  expect(h.getSaved()).toBe(revision);expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(1)
  expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:original,version:revision})
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(1)
})
test('undo conflict preserves the current canvas and never reloads a rejected rollback',async()=>{
  const h=setup();await applyProposal(h);h.conflict()
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:false,error:'conflict',view:{revision:hash(edited)}})
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
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(await h.controller.request({action:'close'})).toMatchObject({ok:false,error:'unknown'})
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{revision,canUndo:false,saving:false}})
  expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(1);expect(h.getStatusCalls()).toBe(1)
})
test('a failed undo reload remains recoverable and never repeats the committed rollback',async()=>{
  const h=setup();await applyProposal(h);h.failReload()
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{revision,canUndo:false,saving:false}})
  expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(1);expect(h.getStatusCalls()).toBe(0)
})

test('definitively rejected undo releases pending state and can be retried or closed',async()=>{
  const h=setup();await applyProposal(h);h.rejectCommit()
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:false,error:'save_failed',view:{saving:false,canUndo:true}})
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:true,view:{canUndo:false,revision}})
  expect(h.getStatusCalls()).toBe(0)
  expect(await h.controller.request({action:'close'})).toMatchObject({ok:true,view:null})
})
test('undo reload rechecks current file rather than treating a historical receipt as current state',async()=>{
  const h=setup();await applyProposal(h);h.failReload()
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:false,error:'unknown'})
  h.externalEdit()
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'conflict',view:{saving:false,status:'error',canUndo:false}})
  expect(h.getSaved()).toBe(hash(edited));expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(1)
  expect(await h.controller.request(open)).toMatchObject({ok:true,view:{revision:hash(edited),status:'ready'}})
})


test('an unapproved native change cannot save and remains protected from accidental close',async()=>{
  const h=setup();await h.controller.request(open)
  await h.controller.request({action:'annotate',...h.target()});h.change()
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'invalid_request',view:{dirty:true}})
  expect(await h.controller.request({action:'close'})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(h.getCommitCalls()).toBe(0)
})
test.each([false,true])('Core-owned recovery survives close/reopen (new controller: %s)',async(restart)=>{
  const h=setup();expect((await applyProposal(h)).ok).toBe(true)
  expect(await h.controller.request({action:'close'})).toEqual({ok:true,view:null})
  if(restart)h.restart()
  expect(await h.controller.request(open)).toMatchObject({ok:true,view:{canUndo:true,recovery:{current:{changeId,threadId,status:'committed',beforeText:'selected raw field',afterText:'proposed raw field'}}}})
  expect(h.controller.getView()?.appliedProposals).toBeUndefined()
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:true,view:{revision,canUndo:false,recovery:{current:{status:'undone'}}}})
  expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(1)
  expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:original,version:revision})
})
test('switching threads clears proposal raw reviews and restores only the owning thread recovery',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()})
  await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:true})
  expect(await h.controller.request({action:'proposals',objectId,scopeId:h.scopeId})).toMatchObject({ok:true,view:{localReviews:[{beforeText:'selected raw field'}]}})
  expect((await h.controller.request({...open,threadId:'other-thread'})).ok).toBe(true)
  expect(h.controller.getView()?.localReviews).toBeUndefined()
  expect(h.controller.getView()?.scope).toBeUndefined()
  expect(JSON.stringify(h.controller.getView())).not.toContain('selected raw field')
  expect((await applyProposal(h)).ok).toBe(true)
  expect(await h.controller.request({...open,threadId:'other-thread'})).toMatchObject({ok:true,view:{canUndo:false,recovery:{current:null,pending:null}}})
  expect(JSON.stringify(h.controller.getView())).not.toContain('selected raw field')
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(h.getUndoCalls()).toBe(0)
  expect(await h.controller.request(open)).toMatchObject({ok:true,view:{canUndo:true,recovery:{current:{threadId}}}})
})
test('a foreign-thread Core recovery record is rejected without exposing its raw review',async()=>{
  const h=setup();await applyProposal(h);h.foreignRecovery()
  expect(await h.controller.request({...open,threadId:'other-thread'})).toMatchObject({ok:false,error:'invalid_response'})
  expect(JSON.stringify(h.controller.getView())).not.toContain('selected raw field')
  expect(h.controller.getView()?.canUndo).toBe(false)
})
test('mismatched open bytes and committed Core digest never advance the native canvas',async()=>{
  const unopened=setup();unopened.corruptOpen()
  expect(await unopened.controller.request(open)).toMatchObject({ok:false,error:'invalid_response',view:null})
  expect(unopened.requests).toHaveLength(0)
  const h=setup();h.corruptReceipt()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'invalid_response',view:{revision,dirty:true,canUndo:false}})
  expect(h.requests.some(r=>r.command==='ack'&&r.status==='committed')).toBe(false)
})


test('a newly created controller completes persisted UndoStarted without a remembered Main operation',async()=>{
  const h=setup();await applyProposal(h)
  await h.controller.request({action:'close'});h.markUndoStarted();h.restart()
  expect(await h.controller.request(open)).toMatchObject({ok:true,view:{canUndo:true,recovery:{current:{status:'unknown',canUndo:false,canRetryUndo:true}}}})
  expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:true,view:{revision,canUndo:false,recovery:{current:{status:'undone',canRetryUndo:false}}}})
  expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(1)
})
test('cancelling a confirmed uncommitted proposal discards only the workcopy and reloads Core bytes',async()=>{
  const h=setup();h.rejectCommit()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'save_failed',view:{dirty:true}})
  expect(await h.controller.request({action:'cancelChange',threadId,changeId,...h.target()})).toMatchObject({ok:true,view:{revision,dirty:false,recovery:{current:null,pending:null}}})
  expect(h.controller.getView()?.editing).not.toBe(true)
  expect(h.getSaved()).toBe(revision);expect(h.getCancelCalls()).toBe(1)
  expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(0)
  expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:original,version:revision})
})
test('an uncertain proposal cannot be cancelled by Main or a different thread',async()=>{
  const h=setup();h.loseDecision()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'unavailable'})
  h.markUncertain()
  expect(await h.controller.request({action:'cancelChange',threadId,changeId,...h.target()})).toMatchObject({ok:false,error:'unknown'})
  expect(await h.controller.request({action:'cancelChange',threadId:'other-thread',changeId,...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(h.getCancelCalls()).toBe(0);expect(h.getCommitCalls()).toBe(0)
  expect(h.getSaved()).toBe(revision)
})
