import { createHash } from 'node:crypto'
import { expect, test } from 'vitest'
import { createNativeOfficeController, type NativeOfficeEngineRequest } from './native-office-controller'
import type { PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'
const hash = (value: Uint8Array) => createHash('sha256').update(value).digest('hex')
const original = new Uint8Array([80,75,1]), edited = new Uint8Array([80,75,2])
const objectId = 'a'.repeat(64), sessionId = 'e'.repeat(48), revision = hash(original)
const threadId = 'thread-native', changeId = '9'.repeat(64), saveOperationId = 'save_core_0001', undoOperationId = 'undo_core_0001'
const open = {action:'open',threadId,workspace:'/workspace',path:'a.docx',bounds:{x:0,y:0,width:800,height:600}}
function setup(typed = false, presentation = false) {
  const presentationSelection={pageIndex:0,targetShapeIndex:0,pageWidth100thMm:1000,pageHeight100thMm:1000,shapes:[{shapeIndex:0,kind:'rectangle' as const,name:'shape',text:'selected',x100thMm:100,y100thMm:100,width100thMm:100,height100thMm:100,fillRGB:'#123456'}]}
  const presentationReview={before:presentationSelection,after:{...presentationSelection,shapes:[{...presentationSelection.shapes[0],fillRGB:'#abcdef'}]}}
  let failTyped=false, rejectNoop=false, rejectText=false, rejectFidelity=false
  const workbook={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0,cells:[{sheet:0,column:0,row:0,text:'1',formula:'1',value:1,valueType:'number' as const,numberFormat:0,rowVisible:true as const,columnVisible:true as const,merged:false as const}]}
  const workbookReview={before:workbook,after:{...workbook,cells:[{...workbook.cells[0],text:'2',formula:'2',value:2}]},results:['']}
  let current = {documentId:objectId,version:revision,kind:(presentation?'pptx':typed?'xlsx':'docx') as 'pptx'|'xlsx'|'docx',changeSequence:0,acknowledgedSequence:0,dirty:false}
  let onEvent:(event:unknown)=>void = ()=>{}, opened = '', saved = revision, commitCalls=0, undoCalls=0, cancelCalls=0, resumeCalls=0, statusCalls=0
  let receipt:any, deferCommit:((value:any)=>void)|undefined
  let delay=false, conflict=false, hidden=0, destroyed=0, revoked=0, loseCommit=false, failReload=false, rejectCommit=false
  const scopeId='d'.repeat(48), proposalId='f'.repeat(48)
  let persisted:any, foreignRecovery=false, corruptOpen=false, corruptReceipt=false, corruptReload=false
  let interruptSave=false, interruptResume=false, interruptUndo=false, loseResumeReceipt=false, pendingCandidate:Uint8Array|undefined
  const statusOperationIds:string[]=[]
  const receipts = new Map<string, any>()
  let scope:any, loseDecision=false, acceptedOperation:string|undefined, replacementText='proposed'
  let openGate:Promise<void>|undefined, releaseOpen:(()=>void)|undefined
  const pkg:PluginPackageView={packageId:presentation?'analytix-presentations':typed?'analytix-spreadsheets':'analytix-documents',packageVersion:'1.0.0',displayName:'Documents',origin:'development-source',publishable:false,materialized:true,generationId:'b'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'c'.repeat(64),available:true,operations:['open-object','close-object','commit-object','object-status','capture-selection','proposal-read','proposal-accept','proposal-reject','selection-revoke','object-recovery','undo-change','cancel-change','resume-change']}
  const selection=()=>presentation?({documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'shapes',scope:'page-and-shape-index-at-version-and-change-sequence',shapes:[{pageIndex:0,shapeIndex:0,name:'shape',type:'com.sun.star.drawing.RectangleShape',text:'selected'}],presentation:presentationSelection,token:'selection_123',capture:{capturedCharacters:8,totalCharacters:8,truncated:false,unit:'utf-16',complete:true}}):typed?({documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'cells',scope:'sheet-range-address-at-version-and-change-sequence',text:'1',ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}],cells:workbook.cells,token:'selection_123',capture:{capturedCharacters:1,totalCharacters:1,truncated:false,unit:'utf-16',complete:true}}):({documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'text',scope:'session-text-range-at-version-and-change-sequence',text:'selected',token:'selection_123',capture:{capturedCharacters:8,totalCharacters:8,truncated:false,unit:'utf-16',complete:true}})
  const requests:NativeOfficeEngineRequest[]=[]
  const createController=()=>createNativeOfficeController({
    async packageHost(r):Promise<any>{
      if(r.action==='list')return {ok:true,packages:[pkg]}
      if(r.action!=='invoke')throw Error('not authorized')
      if(r.operation==='open-object'){await openGate;return {ok:true,output:{ok:true,document:{objectId,sessionId,path:presentation?'/workspace/a.pptx':typed?'/workspace/a.xlsx':'/workspace/a.docx',revision:saved,content:Buffer.from(corruptReload?original:corruptOpen?edited:saved===revision?original:edited).toString('base64')}}}}
      if(r.operation==='capture-selection'){scope={...r.input,scopeId,parts:[{kind:'literal',text:'selected'}]};delete scope.text;return {ok:true,output:{ok:true,scope}}}
      if(r.operation==='proposal-read')return {ok:true,output:{ok:true,proposals:[{proposalId,status:'proposed',parts:[{kind:'literal',text:'proposed'}]}],localReviews:[{proposalId,beforeText:'selected raw field',afterText:'proposed raw field'}]}}
      if(r.operation==='object-recovery'){
        const owned=persisted && (persisted.threadId===r.input.threadId || foreignRecovery) ? structuredClone(persisted) : null
        return {ok:true,output:{ok:true,recovery:{current:owned?.revision&&!pendingCandidate?owned:null,pending:owned&&(!owned.revision||pendingCandidate)?owned:null}}}
      }
      if(r.operation==='cancel-change'){
        cancelCalls++
        expect(r.input).toEqual({sessionId,threadId,changeId,baseRevision:saved})
        expect(persisted.canCancel).toBe(true);expect(persisted.revision).toBe('')
        persisted=undefined;pendingCandidate=undefined
        return {ok:true,output:{ok:true,recovery:{current:null,pending:null}}}
      }
      if(r.operation==='selection-revoke'){revoked++;return {ok:true,output:{ok:true,revoked:true}}}
      if(r.operation==='proposal-accept'){
        if(rejectNoop)return {ok:true,output:{ok:false,code:'proposal_invalid',message:'The proposal does not change the selected text.'}}
        if(acceptedOperation)expect(r.input.operationId).toBe(acceptedOperation)
        acceptedOperation=r.input.operationId as string
        persisted ??= {changeId,threadId:scope.threadId,proposalId,baseRevision:scope.baseRevision,revision:'',status:'prepared',beforeText:'selected raw field',afterText:'proposed raw field',saveOperationId,undoOperationId,canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'2026-09-15T00:00:00Z',savedAt:''}
        if(loseDecision){loseDecision=false;throw Error('transport lost after Core accepted')}
        return {ok:true,output:{ok:true,replacement:{proposalId,changeId,saveOperationId,operationId:r.input.operationId,text:replacementText,...(presentation?{presentation:presentationReview}:typed?{workbook:workbookReview}:{}),selectionToken:scope.selectionToken,changeSequence:scope.changeSequence,baseRevision:scope.baseRevision}}}
      }
      if(r.operation==='close-object')return {ok:true,output:{ok:true,closed:true}}
      if(r.operation==='commit-object'){
        commitCalls++;const input=r.input as any
        expect(input).toMatchObject({changeId:persisted.changeId,threadId:persisted.threadId,operationId:persisted.saveOperationId})
        if(rejectCommit){rejectCommit=false;return {ok:true,output:{ok:false,code:'not_text',message:'unsupported'}}}
        const content = new Uint8Array(Buffer.from(input.content.data, 'base64'))
        expect(input.content).toEqual({encoding:'base64',kind:presentation?'pptx':typed?'xlsx':'docx',data:Buffer.from(content).toString('base64'),sha256:hash(content),byteLength:3})
        expect(input.baseRevision).toBe(saved)
        if(interruptSave){
          interruptSave=false;pendingCandidate=content.slice()
          persisted={...persisted,status:'unknown',revision:hash(content),canUndo:false,canCancel:false,canResume:true}
          receipts.set(saveOperationId,{operationId:saveOperationId,revision:'',status:'pending',savedAt:''})
          throw Error('transport lost after durable candidate, before replacing original')
        }
        receipt={operationId:input.operationId,revision:conflict?'':corruptReceipt?revision:hash(content),status:conflict?'conflict':'committed',savedAt:new Date().toISOString()}
        if(!conflict)saved=hash(content)
        persisted={...persisted,status:conflict?'conflict':'committed',revision:conflict?'':hash(content),savedAt:receipt.savedAt,canUndo:!conflict,canCancel:false}
        receipts.set(input.operationId,receipt)
        if(loseCommit){loseCommit=false;throw Error('reply lost after commit')}
        if(delay)return await new Promise(resolve=>{deferCommit=resolve})
        return {ok:true,output:{ok:true,receipt}}
      }
      if(r.operation==='resume-change'){
        resumeCalls++
        expect(r.input).toEqual({sessionId,threadId,changeId,baseRevision:revision})
        expect(persisted.canResume).toBe(true)
        expect(pendingCandidate).toEqual(edited)
        expect(hash(pendingCandidate!)).toBe(persisted.revision)
        expect(saved).toBe(persisted.baseRevision)
        if(interruptResume){interruptResume=false;throw Error('resume interrupted before replacing original')}
        saved=hash(pendingCandidate!);pendingCandidate=undefined
        persisted={...persisted,status:'committed',canResume:false,canUndo:true,savedAt:'2026-09-15T00:02:00Z'}
        receipt={operationId:saveOperationId,revision:corruptReceipt?revision:saved,status:'committed',savedAt:persisted.savedAt}
        receipts.set(saveOperationId,receipt)
        if(loseResumeReceipt){loseResumeReceipt=false;throw Error('resume response lost after persistence')}
        return {ok:true,output:{ok:true,receipt}}
      }
      if(r.operation==='undo-change'){
        undoCalls++
        expect(r.input).toEqual({sessionId,threadId,changeId,baseRevision:hash(edited)})
        if(interruptUndo){
          interruptUndo=false;persisted={...persisted,status:'unknown',canUndo:false,canRetryUndo:true}
          receipts.set(undoOperationId,{operationId:undoOperationId,revision:'',status:'pending',savedAt:''})
          throw Error('undo interrupted before rollback journal completed')
        }
        if(rejectCommit){rejectCommit=false;return {ok:true,output:{ok:false,code:'not_text',message:'unsupported'}}}
        receipt={operationId:undoOperationId,revision:conflict?'':revision,status:conflict?'conflict':'committed',savedAt:'2026-09-15T00:01:00Z'}
        receipts.set(undoOperationId,receipt)
        if(!conflict){saved=revision;persisted={...persisted,status:'undone',canUndo:false,canRetryUndo:false}}
        if(loseCommit){loseCommit=false;throw Error('reply lost after undo')}
        return {ok:true,output:{ok:true,receipt}}
      }
      if(r.operation==='object-status'){statusCalls++;statusOperationIds.push(r.input.operationId as string);return {ok:true,output:{ok:true,receipt:receipts.get(r.input.operationId as string)}}}
      throw Error('unexpected Core operation')
    },
    createSurface(listener){onEvent=listener;return {attach(){},hide(){hidden++},destroy(){destroyed++},async request(r){
      requests.push(r)
      const base={type:'result',channel:'surface',command:r.command,operationId:r.operationId,documentId:r.documentId,version:r.version,ok:true}
      if(r.command==='open'){if(failReload){failReload=false;throw Error('reload failed')}opened=r.operationId;current={...current,version:r.version,changeSequence:0,acknowledgedSequence:0,dirty:false}}
      if(r.command==='close')return base
      if(r.command==='edit'&&rejectFidelity)throw Error('unsupported-format-fidelity')
      if(r.command==='replace'&&rejectText)throw Error('unsupported-selection')
      if((r.command==='replaceCells'||r.command==='replacePresentation')&&failTyped){current={...current,changeSequence:1,dirty:true};throw Error('typed-mutation-failed')}
      if(r.command==='replace'||r.command==='replaceCells'||r.command==='replacePresentation')current={...current,changeSequence:current.changeSequence+1,dirty:true}
      if(r.command==='export')return {...base,state:{...current},bytes:edited,exportedSequence:current.changeSequence}
      if(r.command==='ack'&&r.status==='committed')current={...current,version:r.persistedVersion!,acknowledgedSequence:r.exportedSequence!,dirty:current.changeSequence!==r.exportedSequence}
      return {...base,state:{...current},selection:selection()}
    }}},onChange(){}})
  let controller=createController()
  const change=()=>{current={...current,changeSequence:current.changeSequence+1,dirty:true};onEvent({type:'changed',operationId:opened,documentId:objectId,version:current.version,state:{...current}})}
  const target=()=>({objectId,revision:current.version,expectedChangeSequence:current.changeSequence})
  return {rejectFidelity:()=>{rejectFidelity=true},rejectText:()=>{rejectText=true},rejectNoop:()=>{rejectNoop=true},getPrepared:()=>persisted,failTyped:()=>{failTyped=true},get controller(){return controller},restart:()=>{controller=createController()},corruptOpen:()=>{corruptOpen=true},corruptReceipt:()=>{corruptReceipt=true},foreignRecovery:()=>{foreignRecovery=true},interruptResume:()=>{interruptResume=true},interruptUndo:()=>{interruptUndo=true},mismatchRecoveryOperation:()=>{persisted.saveOperationId='other_save_0001';persisted.undoOperationId='other_undo_0001'},getResumeCalls:()=>resumeCalls,getStatusOperationIds:()=>[...statusOperationIds],interruptSave:()=>{interruptSave=true},loseResumeReceipt:()=>{loseResumeReceipt=true},corruptReload:()=>{corruptReload=true},wrongPendingBase:()=>{persisted.baseRevision='0'.repeat(64)},getUndoCalls:()=>undoCalls,getCancelCalls:()=>cancelCalls,markUndoStarted:()=>{persisted={...persisted,status:'unknown',canUndo:false,canRetryUndo:true}},markUncertain:()=>{persisted={...persisted,status:'unknown',canCancel:false}},target,change,requests,pkg,scopeId,proposalId,getRevoked:()=>revoked,getSaved:()=>saved,externalEdit:()=>{saved=hash(edited)},rejectCommit:()=>{rejectCommit=true},loseCommit:()=>{loseCommit=true},failReload:()=>{failReload=true},loseDecision:()=>{loseDecision=true},oversize:()=>{replacementText='x'.repeat(4097)},holdOpen:()=>{openGate=new Promise(resolve=>{releaseOpen=resolve})},releaseOpen:()=>releaseOpen?.(),delay:()=>{delay=true},conflict:()=>{conflict=true},complete:()=>deferCommit?.({ok:true,output:{ok:true,receipt}}),getCommitCalls:()=>commitCalls,getStatusCalls:()=>statusCalls,getHidden:()=>hidden,getDestroyed:()=>destroyed}
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
test('Core rejection of an unchanged proposal never replaces, exports, or commits native bytes',async()=>{
  const h=setup();h.rejectNoop()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'unavailable',view:{dirty:false,changeSequence:0,revision}})
  expect(h.requests.filter(r=>r.command==='replace'||r.command==='replaceCells'||r.command==='export')).toHaveLength(0)
  expect(h.getPrepared()).toBeUndefined()
  expect(h.getCommitCalls()).toBe(0)
  expect(h.getSaved()).toBe(revision)
  expect(h.controller.getView()?.appliedProposals ?? []).not.toContain(h.proposalId)
})
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


async function pendingSaveAfterRestart(h:ReturnType<typeof setup>){
  h.interruptSave()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'unknown',view:{dirty:true,saving:true}})
  expect(h.getSaved()).toBe(revision)
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'unknown'})
  expect(h.getSaved()).toBe(revision);expect(h.getCommitCalls()).toBe(1);expect(h.getResumeCalls()).toBe(0)
  // Simulate losing Main entirely; only the fake Core journal and candidate survive.
  h.restart()
  expect(await h.controller.request(open)).toMatchObject({ok:true,view:{revision,dirty:false,recovery:{pending:{changeId,baseRevision:revision,revision:hash(edited),canResume:true}}}})
}
test('an unknown save survives a new controller and only explicit resume publishes the durable candidate once',async()=>{
  const h=setup();await pendingSaveAfterRestart(h)
  const engineCount=h.requests.length
  for(let i=0;i<2;i++)expect((await h.controller.request({action:'saveStatus',objectId})).ok).toBe(true)
  expect(h.getSaved()).toBe(revision);expect(h.getResumeCalls()).toBe(0);expect(h.getCommitCalls()).toBe(1)
  const request={action:'resumeChange',threadId,changeId,...h.target()}
  expect(await h.controller.request(request)).toMatchObject({ok:true,view:{revision:hash(edited),dirty:false,saving:false,canUndo:true,recovery:{current:{status:'committed',canResume:false},pending:null}}})
  expect(h.requests.slice(engineCount).filter(r=>r.command==='replace'||r.command==='export')).toHaveLength(0)
  expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:edited,version:hash(edited)})
  expect(await h.controller.request(request)).toMatchObject({ok:false,error:'stale_selection'})
  expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(h.getResumeCalls()).toBe(1);expect(h.getCommitCalls()).toBe(1)
  await h.controller.request({action:'close'});h.restart()
  expect(await h.controller.request(open)).toMatchObject({ok:true,view:{revision:hash(edited),canUndo:true,recovery:{current:{changeId,status:'committed'},pending:null}}})
})
test('a lost resume receipt is queried by the original save operation without another write or engine export',async()=>{
  const h=setup();await pendingSaveAfterRestart(h);h.loseResumeReceipt()
  const engineCount=h.requests.length
  expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target()})).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target()})).toMatchObject({ok:false,error:'unknown'})
  expect(h.getResumeCalls()).toBe(1)
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{revision:hash(edited),saving:false,dirty:false,canUndo:true}})
  expect(h.getStatusOperationIds()).toEqual([saveOperationId,saveOperationId])
  expect(h.getResumeCalls()).toBe(1);expect(h.getCommitCalls()).toBe(1)
  expect(h.requests.slice(engineCount).filter(r=>r.command==='replace'||r.command==='export')).toHaveLength(0)
  await h.controller.request({action:'close'});h.restart()
  expect(await h.controller.request(open)).toMatchObject({ok:true,view:{revision:hash(edited),recovery:{current:{changeId},pending:null}}})
})
test('resume rejects another thread, stale canvas version and a mismatched Core base revision',async()=>{
  const h=setup();await pendingSaveAfterRestart(h)
  expect(await h.controller.request({action:'resumeChange',threadId:'other-thread',changeId,...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target(),revision:'0'.repeat(64)})).toMatchObject({ok:false,error:'stale_selection'})
  expect(await h.controller.request({action:'resumeChange',threadId,changeId:'8'.repeat(64),...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target(),content:'untrusted replacement bytes'})).toMatchObject({ok:false,error:'invalid_request'})
  h.wrongPendingBase()
  expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(h.getResumeCalls()).toBe(0);expect(h.getSaved()).toBe(revision)
})
test.each(['receipt','reloaded bytes'] as const)('resume rejects an incorrect %s digest without reloading unverified content',async(kind)=>{
  const h=setup();await pendingSaveAfterRestart(h)
  if(kind==='receipt')h.corruptReceipt();else h.corruptReload()
  const opens=h.requests.filter(r=>r.command==='open').length
  expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target()})).toMatchObject({ok:false,error:'invalid_response',view:{revision,saving:true}})
  expect(h.requests.filter(r=>r.command==='open')).toHaveLength(opens)
  expect(h.getResumeCalls()).toBe(1);expect(h.getCommitCalls()).toBe(1)
})


test('a pending-save query refreshes explicit recovery capability without writing or exporting',async()=>{
  const h=setup();h.interruptSave()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'unknown',view:{dirty:true,saving:true}})
  expect(h.controller.getView()?.recovery?.pending).toBeNull()
  const engineCount=h.requests.length
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'unknown',view:{dirty:true,saving:true,recovery:{pending:{changeId,canResume:true}}}})
  expect(h.getSaved()).toBe(revision)
  expect(h.getCommitCalls()).toBe(1);expect(h.getResumeCalls()).toBe(0);expect(h.getUndoCalls()).toBe(0)
  expect(h.requests).toHaveLength(engineCount)
  expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target()})).toMatchObject({ok:true,view:{revision:hash(edited),dirty:false,saving:false}})
  expect(h.getCommitCalls()).toBe(1);expect(h.getResumeCalls()).toBe(1)
})
test('querying an interrupted resume unlocks the same operation for a later explicit click only',async()=>{
  const h=setup();await pendingSaveAfterRestart(h);h.interruptResume()
  const resume={action:'resumeChange',threadId,changeId,...h.target()}
  expect(await h.controller.request(resume)).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(await h.controller.request(resume)).toMatchObject({ok:false,error:'unknown'})
  const engineCount=h.requests.length
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'unknown',view:{saving:false,recovery:{pending:{canResume:true,saveOperationId}}}})
  expect(h.getSaved()).toBe(revision);expect(h.getResumeCalls()).toBe(1)
  expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(0)
  expect(h.requests).toHaveLength(engineCount)
  expect(await h.controller.request(resume)).toMatchObject({ok:true,view:{revision:hash(edited),saving:false}})
  expect(h.getResumeCalls()).toBe(2);expect(h.getCommitCalls()).toBe(1)
})
test('querying an interrupted undo exposes retry but never rolls back until the next explicit click',async()=>{
  const h=setup();await applyProposal(h);h.interruptUndo()
  const undo={action:'undoChange',threadId,...h.target()}
  expect(await h.controller.request(undo)).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(await h.controller.request(undo)).toMatchObject({ok:false,error:'unknown'})
  const engineCount=h.requests.length
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'unknown',view:{saving:false,canUndo:true,recovery:{current:{canRetryUndo:true,undoOperationId}}}})
  expect(h.getSaved()).toBe(hash(edited));expect(h.getUndoCalls()).toBe(1)
  expect(h.getResumeCalls()).toBe(0);expect(h.getCommitCalls()).toBe(1)
  expect(h.requests).toHaveLength(engineCount)
  expect(await h.controller.request(undo)).toMatchObject({ok:true,view:{revision,saving:false,canUndo:false}})
  expect(h.getUndoCalls()).toBe(2);expect(h.getCommitCalls()).toBe(1)
})
test.each(['resume','undo'] as const)('a query never unlocks a pending %s using another Core operation capability',async(kind)=>{
  const h=setup()
  if(kind==='resume'){await pendingSaveAfterRestart(h);h.interruptResume()}else{await applyProposal(h);h.interruptUndo()}
  const request=kind==='resume'?{action:'resumeChange',threadId,changeId,...h.target()}:{action:'undoChange',threadId,...h.target()}
  expect(await h.controller.request(request)).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  h.mismatchRecoveryOperation()
  const engineCount=h.requests.length, saved=h.getSaved()
  expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(await h.controller.request(request)).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
  expect(h.getSaved()).toBe(saved);expect(h.requests).toHaveLength(engineCount)
  expect(h.getResumeCalls()).toBe(kind==='resume'?1:0);expect(h.getUndoCalls()).toBe(kind==='undo'?1:0)
  expect(h.getCommitCalls()).toBe(1)
})


test('typed approval uses only replaceCells and the fixed Core save operation',async()=>{
 const h=setup(true);expect((await h.controller.request({...open,path:'a.xlsx'})).ok).toBe(true)
 const result=await applyProposal(h);expect(result.ok,JSON.stringify(result)).toBe(true)
 expect(h.requests.filter(r=>r.command==='replaceCells')).toHaveLength(1)
 expect(h.requests.some(r=>r.command==='replace')).toBe(false)
 expect(h.getCommitCalls()).toBe(1)
})
test('partial typed mutation destroys poisoned workcopy and reloads Core original before saveAll',async()=>{
 const h=setup(true);expect((await h.controller.request({...open,path:'a.xlsx'})).ok).toBe(true);h.failTyped()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'save_failed',view:{dirty:false,editing:false,revision}})
 expect(h.getDestroyed()).toBe(1)
 expect(h.requests.filter(r=>r.command==='open')).toHaveLength(2)
 expect(h.requests.at(-1)?.command).not.toBe('export')
 await h.controller.saveAll()
 expect(h.requests.some(r=>r.command==='export')).toBe(false)
 expect(h.getCommitCalls()).toBe(0)
})

test('presentation approval uses typed worker command and the existing Core save receipt',async()=>{
 const h=setup(false,true);expect((await h.controller.request({...open,path:'a.pptx'})).ok).toBe(true)
 expect(await applyProposal(h)).toMatchObject({ok:true})
 expect(h.requests.filter(r=>r.command==='replacePresentation')).toHaveLength(1)
 expect(h.requests.some(r=>r.command==='replace')).toBe(false)
 expect(h.getCommitCalls()).toBe(1)
})
test('partially changed presentation is destroyed and original reopened without exporting',async()=>{
 const h=setup(false,true);expect((await h.controller.request({...open,path:'a.pptx'})).ok).toBe(true);h.failTyped()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'save_failed',view:{dirty:false,editing:false,revision}})
 await h.controller.saveAll()
 expect(h.getDestroyed()).toBe(1);expect(h.requests.filter(r=>r.command==='open')).toHaveLength(2)
 expect(h.requests.some(r=>r.command==='export')).toBe(false);expect(h.getCommitCalls()).toBe(0)
})


test('native structure rejection retains the prepared review without exporting or saving bytes',async()=>{
  const h=setup();h.rejectText()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'unsupported_selection',view:{dirty:false,changeSequence:0,revision}})
  expect(h.requests.filter(r=>r.command==='export')).toHaveLength(0)
  expect(h.getCommitCalls()).toBe(0);expect(h.getSaved()).toBe(revision)
  expect(h.getPrepared()).toMatchObject({status:'prepared',canCancel:true,canResume:false,revision:''})
  expect(h.controller.getView()?.appliedProposals ?? []).not.toContain(h.proposalId)
  await h.controller.request({action:'saveStatus',objectId:h.target().objectId})
  expect(await h.controller.request({action:'cancelChange',threadId,changeId,...h.target()})).toMatchObject({ok:true})
  expect(h.getSaved()).toBe(revision);expect(h.getCommitCalls()).toBe(0)
})


test('unsupported DOCX format remains previewable without edit authority or a Core save',async()=>{
  const h=setup();h.rejectFidelity()
  expect(await h.controller.request(open)).toMatchObject({ok:true})
  expect(await h.controller.request({action:'annotate',...h.target()})).toMatchObject({ok:false,error:'unsupported_selection',view:{dirty:false,revision}})
  expect(h.controller.getView()?.editing).not.toBe(true)
  expect(h.requests.some(r=>r.command==='replace'||r.command==='export')).toBe(false)
  expect(h.getCommitCalls()).toBe(0);expect(h.getSaved()).toBe(revision)
  expect(await h.controller.request({action:'close',objectId})).toMatchObject({ok:true})
})
