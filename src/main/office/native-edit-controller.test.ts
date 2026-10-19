import { createHash } from 'node:crypto'
import { expect, test, vi } from 'vitest'
import * as logger from '../logger'
import { createNativeOfficeController, type NativeOfficeEngineRequest } from './native-office-controller'
import type { PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'
import type { NativeOfficeView } from '../../shared/native-office'
const hash = (value: Uint8Array) => createHash('sha256').update(value).digest('hex')
const original = new Uint8Array([80,75,1]), edited = new Uint8Array([80,75,2])
const externallyEdited = new Uint8Array([80,75,3])
const objectId = 'a'.repeat(64), sessionId = 'e'.repeat(48), revision = hash(original)
const threadId = 'thread-native', changeId = '9'.repeat(64), saveOperationId = 'save_core_0001', undoOperationId = 'undo_core_0001'
const open = {action:'open',threadId,workspace:'/workspace',path:'a.docx',bounds:{x:0,y:0,width:800,height:600}}
function setup(typed = false, presentation = false) {
  const openRequest = { ...open, path: presentation ? 'a.pptx' : typed ? 'a.xlsx' : 'a.docx' }
  const presentationSelection={pageIndex:0,targetShapeIndex:0,pageWidth100thMm:1000,pageHeight100thMm:1000,shapes:[{shapeIndex:0,kind:'rectangle' as const,name:'shape',text:'selected',x100thMm:100,y100thMm:100,width100thMm:100,height100thMm:100,fillRGB:'#123456'}]}
  const presentationReview={before:presentationSelection,after:{...presentationSelection,shapes:[{...presentationSelection.shapes[0],fillRGB:'#abcdef'}]}}
  let failTyped=false, busyTyped=false, rejectNoop=false, rejectText=false, rejectFidelity=false
  const workbook={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0,cells:[{sheet:0,column:0,row:0,text:'1',formula:'1',value:1,valueType:'number' as const,numberFormat:0,rowVisible:true as const,columnVisible:true as const,merged:false as const}]}
  const workbookReview={before:workbook,after:{...workbook,cells:[{...workbook.cells[0],text:'2',formula:'2',value:2}]},results:['']}
  let current = {documentId:objectId,version:revision,kind:(presentation?'pptx':typed?'xlsx':'docx') as 'pptx'|'xlsx'|'docx',changeSequence:0,acknowledgedSequence:0,dirty:false}
  let onEvent:(event:unknown)=>void = ()=>{}, opened = '', saved = revision, commitCalls=0, undoCalls=0, cancelCalls=0, resumeCalls=0, statusCalls=0
  let receipt:any, deferCommit:((value:any)=>void)|undefined
  let delay=false, conflict=false, hidden=0, destroyed=0, revoked=0, loseCommit=false, failReload=false, rejectCommit=false
  const scopeId='d'.repeat(48), proposalId='f'.repeat(48)
  let persisted:any, foreignRecovery=false, corruptOpen=false, corruptReceipt=false, corruptReload=false, changeReadback=false
  let interruptSave=false, interruptResume=false, interruptUndo=false, loseResumeReceipt=false, pendingCandidate:Uint8Array|undefined
  const statusOperationIds:string[]=[]
  const receipts = new Map<string, any>()
  let scopeOverride:Record<string,unknown> = {}, selectionReadError=false
  const selectionCalls:string[]=[]
  let scope:any, loseDecision=false, acceptedOperation:string|undefined, replacementText='proposed'
  let candidateOverride:((candidate:any)=>any)|undefined, duringAccept:(()=>void)|undefined
  let preApprovalFailure:'conflict'|'lost'|undefined, recoveryReadFailure=false, recoveryCurrent:any
  let duringRecovery:(()=>void)|undefined
  const decisionInputs:Record<string,unknown>[]=[]
  let openGate:Promise<void>|undefined, releaseOpen:(()=>void)|undefined
  const pkg:PluginPackageView={packageId:presentation?'analytix-presentations':typed?'analytix-spreadsheets':'analytix-documents',packageVersion:'1.0.0',displayName:'Documents',origin:'development-source',publishable:false,materialized:true,generationId:'b'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'c'.repeat(64),available:true,operations:['open-object','close-object','commit-object','object-status','capture-selection','selection-read','proposal-read','proposal-accept','proposal-reject','selection-revoke','object-recovery','undo-change','cancel-change','resume-change']}
  const selection=()=>presentation?({documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'shapes',scope:'page-and-shape-index-at-version-and-change-sequence',shapes:[{pageIndex:0,shapeIndex:0,name:'shape',type:'com.sun.star.drawing.RectangleShape',text:'selected'}],presentation:presentationSelection,token:'selection_123',capture:{capturedCharacters:8,totalCharacters:8,truncated:false,unit:'utf-16',complete:true}}):typed?({documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'cells',scope:'sheet-range-address-at-version-and-change-sequence',text:'1',ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}],cells:workbook.cells,token:'selection_123',capture:{capturedCharacters:1,totalCharacters:1,truncated:false,unit:'utf-16',complete:true}}):({documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'text',scope:'session-text-range-at-version-and-change-sequence',text:'selected',token:'selection_123',capture:{capturedCharacters:8,totalCharacters:8,truncated:false,unit:'utf-16',complete:true}})
  const requests:NativeOfficeEngineRequest[]=[]
  const trace:string[]=[], views:Array<NativeOfficeView|null>=[]
  const createController=()=>createNativeOfficeController({
    async packageHost(r):Promise<any>{
      if(r.action==='list')return {ok:true,packages:[pkg]}
      if(r.action!=='invoke')throw Error('not authorized')
      trace.push('core:'+r.operation)
      if(r.operation==='open-object'){
        await openGate
        if(changeReadback&&saved!==revision){changeReadback=false;saved=hash(externallyEdited)}
        const bytes=corruptReload?original:corruptOpen?edited:saved===revision?original:saved===hash(externallyEdited)?externallyEdited:edited
        return {ok:true,output:{ok:true,document:{objectId,sessionId,path:presentation?'/workspace/a.pptx':typed?'/workspace/a.xlsx':'/workspace/a.docx',revision:saved,content:Buffer.from(bytes).toString('base64')},...(typed?{workbookEditing:{editable:true}}:{})}}
      }
      if(r.operation==='capture-selection'){selectionCalls.push(r.operation);scope={...r.input,scopeId,parts:[{kind:'literal',text:'selected'}],...scopeOverride};delete scope.text;return {ok:true,output:{ok:true,scope}}}
      if(r.operation==='selection-read'){selectionCalls.push(r.operation);return {ok:true,output:selectionReadError?{ok:false,code:'scope_invalid',message:'stale'}:{ok:true,scope:{...scope,...scopeOverride}}}}
      if(r.operation==='proposal-read')return {ok:true,output:{ok:true,proposals:[{proposalId,status:'proposed',parts:[{kind:'literal',text:'proposed'}]}],localReviews:[{proposalId,beforeText:'selected raw field',afterText:'proposed raw field'}]}}
      if(r.operation==='object-recovery'){
        if(decisionInputs.length&&recoveryReadFailure)throw Error('recovery reply lost')
        if(decisionInputs.length)duringRecovery?.()
        const owned=persisted && (persisted.threadId===r.input.threadId || foreignRecovery) ? structuredClone(persisted) : null
        return {ok:true,output:{ok:true,recovery:{current:owned?.revision&&!pendingCandidate?owned:recoveryCurrent??null,pending:owned&&(!owned.revision||pendingCandidate)?owned:null}}}
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
        decisionInputs.push(structuredClone(r.input))
        if(preApprovalFailure){
          const failure=preApprovalFailure;preApprovalFailure=undefined
          if(failure==='lost')throw Error('accept reply lost before approval was discovered')
          return {ok:true,output:{ok:false,code:'conflict',message:'Original changed before approval.'}}
        }
        if(rejectNoop)return {ok:true,output:{ok:false,code:'proposal_invalid',message:'The proposal does not change the selected text.'}}
        if(acceptedOperation)expect(r.input.operationId).toBe(acceptedOperation)
        acceptedOperation=r.input.operationId as string
        persisted ??= {changeId,threadId:scope.threadId,proposalId,baseRevision:scope.baseRevision,revision:'',status:'prepared',beforeText:'selected raw field',afterText:'proposed raw field',...(typed?{workbook:workbookReview}:{}),saveOperationId,undoOperationId,canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'2026-09-15T00:00:00Z',savedAt:''}
        if(loseDecision){loseDecision=false;throw Error('transport lost after Core accepted')}
        if(typed&&failTyped)return {ok:true,output:{ok:false,code:'not_text',message:'candidate generation refused'}}
        const candidate={changeId,saveOperationId,baseRevision:scope.baseRevision,content:{encoding:'base64',kind:'xlsx',byteLength:edited.byteLength,sha256:hash(edited),data:Buffer.from(edited).toString('base64')}}
        duringAccept?.()
        return {ok:true,output:{ok:true,replacement:{proposalId,changeId,saveOperationId,operationId:r.input.operationId,text:replacementText,...(presentation?{presentation:presentationReview}:typed?{workbook:workbookReview,candidate:candidateOverride?candidateOverride(candidate):candidate}:{}),selectionToken:scope.selectionToken,changeSequence:scope.changeSequence,baseRevision:scope.baseRevision}}}
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
    createSurface(listener){trace.push('surface:create');let playbackSequence=0;onEvent=listener;return {attach(){},hide(){hidden++},destroy(){trace.push('surface:destroy');destroyed++},async request(r){
      trace.push('engine:'+r.command+(r.command==='ack'?':'+r.status:''))
      requests.push(r)
      const base={type:'result',channel:'surface',command:r.command,operationId:r.operationId,documentId:r.documentId,version:r.version,ok:true}
      if(r.command==='open'){if(failReload){failReload=false;throw Error('reload failed')}opened=r.operationId;current={...current,version:r.version,changeSequence:0,acknowledgedSequence:0,dirty:false}}
      if(r.command==='close')return base
      if(typed&&['replace','replaceCells','export','ack','manual-edit'].includes(r.command))throw Error('readonly workbook fixture refuses native mutation')
      if(r.command==='local-view'){
        if(current.kind!=='pptx'||r.action!=='play-read')throw Error('unexpected fixture local-view action')
        return {...base,state:{...current},view:{zoom:100,zoomType:3,fit:false,page:1,pages:1,readOnly:true,modified:current.dirty,
          playback:{generation:0,sequence:++playbackSequence,phase:'idle',slideIndex:null,slideCount:null}}}
      }
      if(r.command==='edit'&&rejectFidelity)throw Error('unsupported-format-fidelity')
      if(r.command==='replace'&&rejectText)throw Error('unsupported-selection')
      if((r.command==='replaceCells'||r.command==='replacePresentation')&&busyTyped)throw Error('operation-in-progress')
      if((r.command==='replaceCells'||r.command==='replacePresentation')&&failTyped){current={...current,changeSequence:1,dirty:true};throw Error('typed-mutation-failed')}
      if(r.command==='replace'||r.command==='replaceCells'||r.command==='replacePresentation')current={...current,changeSequence:current.changeSequence+1,dirty:true}
      if(r.command==='export')return {...base,state:{...current},bytes:edited,exportedSequence:current.changeSequence}
      if(r.command==='ack'&&r.status==='committed')current={...current,version:r.persistedVersion!,acknowledgedSequence:r.exportedSequence!,dirty:current.changeSequence!==r.exportedSequence}
      if(typed&&(r.command==='replaceCells'||r.command==='ack'))return {...base,state:{...current}}
      return {...base,state:{...current},selection:selection()}
    }}},onChange(view){views.push(view);trace.push('view:'+(view?.status??'closed'))}})
  let controller=createController()
  const change=()=>{current={...current,changeSequence:current.changeSequence+1,dirty:true};onEvent({type:'changed',operationId:opened,documentId:objectId,version:current.version,state:{...current}})}
  const target=()=>({objectId,revision:current.version,expectedChangeSequence:current.changeSequence})
  return {openRequest,trace,views,decisionInputs,
    refuseBeforeApproval:(value:'conflict'|'lost'='conflict')=>{preApprovalFailure=value},failRecovery:()=>{recoveryReadFailure=true},seedRecoveryCurrent:(value:any)=>{recoveryCurrent=value},duringRecovery:(value:()=>void)=>{duringRecovery=value},
    candidateOverride:(value:(candidate:any)=>any)=>{candidateOverride=value},duringAccept:(value:()=>void)=>{duringAccept=value},changeReadbackAfterCommit:()=>{changeReadback=true},busyTyped:(value:boolean)=>{busyTyped=value},scopeOverride:(value:Record<string,unknown>)=>{scopeOverride=value},refuseSelectionRead:()=>{selectionReadError=true},selectionCalls,rejectFidelity:()=>{rejectFidelity=true},rejectText:()=>{rejectText=true},rejectNoop:()=>{rejectNoop=true},getPrepared:()=>persisted,failTyped:(value=true)=>{failTyped=value},get controller(){return controller},restart:()=>{controller=createController()},corruptOpen:()=>{corruptOpen=true},corruptReceipt:()=>{corruptReceipt=true},foreignRecovery:()=>{foreignRecovery=true},interruptResume:()=>{interruptResume=true},interruptUndo:()=>{interruptUndo=true},mismatchRecoveryOperation:()=>{persisted.saveOperationId='other_save_0001';persisted.undoOperationId='other_undo_0001'},getResumeCalls:()=>resumeCalls,getStatusOperationIds:()=>[...statusOperationIds],interruptSave:()=>{interruptSave=true},loseResumeReceipt:()=>{loseResumeReceipt=true},corruptReload:()=>{corruptReload=true},wrongPendingBase:()=>{persisted.baseRevision='0'.repeat(64)},getUndoCalls:()=>undoCalls,getCancelCalls:()=>cancelCalls,markUndoStarted:()=>{persisted={...persisted,status:'unknown',canUndo:false,canRetryUndo:true}},markUncertain:()=>{persisted={...persisted,status:'unknown',canCancel:false}},target,change,requests,pkg,scopeId,proposalId,getRevoked:()=>revoked,getSaved:()=>saved,externalEdit:()=>{saved=hash(edited)},rejectCommit:()=>{rejectCommit=true},loseCommit:()=>{loseCommit=true},failReload:()=>{failReload=true},loseDecision:()=>{loseDecision=true},oversize:()=>{replacementText='x'.repeat(4097)},holdOpen:()=>{openGate=new Promise(resolve=>{releaseOpen=resolve})},releaseOpen:()=>releaseOpen?.(),delay:()=>{delay=true},conflict:()=>{conflict=true},complete:()=>deferCommit?.({ok:true,output:{ok:true,receipt}}),getCommitCalls:()=>commitCalls,getStatusCalls:()=>statusCalls,getHidden:()=>hidden,getDestroyed:()=>destroyed}
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
  const opened = await h.controller.request(h.openRequest)
  if (!opened.ok) return opened
  const annotated = await h.controller.request({action:'annotate',...h.target()})
  if (!annotated.ok) return annotated
  const referenced = await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:true})
  if (!referenced.ok) return referenced
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
  const diagnostic=vi.spyOn(logger,'logNativeOfficeManualBeginFailure').mockImplementation(()=>undefined)
  try {
    const h=setup();await pendingSaveAfterRestart(h);h.interruptResume()
    const resume={action:'resumeChange',threadId,changeId,...h.target()}
    expect(await h.controller.request(resume)).toMatchObject({ok:false,error:'unknown',view:{saving:true}})
    expect(await h.controller.request({action:'beginManualEdit',threadId,...h.target()})).toMatchObject({ok:false,error:'unknown'})
    expect(diagnostic).toHaveBeenCalledExactlyOnceWith('unknown',{schemaAccepted:null,outerCode:null,innerCode:null,pendingSave:false,pendingRecovery:true,pendingManualCancel:false})
    expect(await h.controller.request(resume)).toMatchObject({ok:false,error:'unknown'})
    const engineCount=h.requests.length
    expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'unknown',view:{saving:false,recovery:{pending:{canResume:true,saveOperationId}}}})
    expect(h.getSaved()).toBe(revision);expect(h.getResumeCalls()).toBe(1)
    expect(h.getCommitCalls()).toBe(1);expect(h.getUndoCalls()).toBe(0)
    expect(h.requests).toHaveLength(engineCount)
    expect(await h.controller.request(resume)).toMatchObject({ok:true,view:{revision:hash(edited),saving:false}})
    expect(h.getResumeCalls()).toBe(2);expect(h.getCommitCalls()).toBe(1)
  } finally {diagnostic.mockRestore()}
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


test('a transient presentation typed apply refusal retains its healthy owner and prepared review for retry',async()=>{
 const h=setup(false,true)
 expect((await h.controller.request(h.openRequest)).ok).toBe(true)
 expect((await h.controller.request({action:'annotate',...h.target()})).ok).toBe(true)
 expect((await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:true})).ok).toBe(true)
 h.busyTyped(true)
 const request={action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId}
 expect(await h.controller.request(request)).toMatchObject({ok:false,error:'unavailable',view:{status:'ready',editing:true,dirty:false,changeSequence:0,revision}})
 expect(h.getDestroyed()).toBe(0);expect(h.requests.filter(r=>r.command==='open')).toHaveLength(1)
 expect(h.requests.some(r=>r.command==='export')).toBe(false);expect(h.getCommitCalls()).toBe(0)
 expect(h.getPrepared()).toMatchObject({status:'prepared',canCancel:true,revision:''})
 expect(h.controller.getView()?.appliedProposals??[]).not.toContain(h.proposalId)
 h.busyTyped(false)
 expect(await h.controller.request(request)).toMatchObject({ok:true,view:{dirty:false,canUndo:true}})
 expect(h.getDestroyed()).toBe(0);expect(h.getCommitCalls()).toBe(1)
 expect(h.requests.filter(r=>r.command==='replacePresentation')).toHaveLength(2)
})
// Direct controller fixtures use synthetic transport bytes and mocked Core/UNO;
// they establish ordering and authority, not native file acceptance.
test('typed Agent approval commits exact Core candidate bytes and reloads before ready without native export or ACK',async()=>{
 const h=setup(true);expect((await h.controller.request({...open,path:'a.xlsx'})).ok).toBe(true)
 const result=await applyProposal(h);expect(result.ok,JSON.stringify(result)).toBe(true)
 expect(result).toMatchObject({ok:true,view:{revision:hash(edited),status:'ready',saving:false,dirty:false,editing:false,changeSequence:0,acknowledgedSequence:0,canUndo:true}})
 expect(h.requests.some(r=>['replace','replaceCells','export','ack','manual-edit'].includes(r.command))).toBe(false)
 expect(h.getPrepared()).toMatchObject({proposalId:h.proposalId,threadId,changeId,status:'committed'})
 expect(h.getPrepared().origin).toBeUndefined();expect(h.getPrepared().version).toBeUndefined()
 expect(h.trace).not.toContain('core:prepare-workbook-manual')
 expect(h.requests.filter(r=>r.command==='open')).toEqual([
  expect.objectContaining({bytes:original,version:revision}),
  expect.objectContaining({bytes:edited,version:hash(edited)})
 ])
 const approval=h.trace.indexOf('core:proposal-accept'),commit=h.trace.indexOf('core:commit-object')
 const readback=h.trace.indexOf('core:open-object',commit+1)
 const retired=h.trace.indexOf('surface:destroy',readback+1),created=h.trace.indexOf('surface:create',retired+1),loaded=h.trace.indexOf('engine:open',created+1)
 expect(approval).toBeGreaterThan(-1);expect(commit).toBeGreaterThan(approval)
 expect(readback).toBeGreaterThan(commit);expect(retired).toBeGreaterThan(readback)
 expect(created).toBeGreaterThan(retired);expect(loaded).toBeGreaterThan(created)
 expect(h.trace.slice(commit+1,loaded)).not.toContain('view:ready')
 expect(h.trace.indexOf('view:ready',loaded+1)).toBeGreaterThan(loaded)
 expect(h.views.some(view=>view?.status==='loading'&&view.saving&&view.canUndo===false)).toBe(true)
 expect(h.views.some(view=>view?.dirty)).toBe(false)
 expect(h.getDestroyed()).toBe(1);expect(h.getCommitCalls()).toBe(1);expect(h.getStatusCalls()).toBe(0)
})
test('typed Agent committed reload failure retries only exact Core readback and surface load',async()=>{
 const h=setup(true);expect((await h.controller.request(h.openRequest)).ok).toBe(true);h.failReload()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'unknown',view:{saving:true,canUndo:false,status:'error'}})
 expect(h.getSaved()).toBe(hash(edited));expect(h.getPrepared()).toMatchObject({proposalId:h.proposalId,status:'committed'})
 expect(h.getCommitCalls()).toBe(1);expect(h.requests.filter(r=>r.command==='export')).toHaveLength(0)
 const beforeRetry=h.trace.length
 expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{revision:hash(edited),status:'ready',saving:false,dirty:false,editing:false,canUndo:true}})
 expect(h.trace.slice(beforeRetry)).toContain('core:open-object')
 expect(h.trace.slice(beforeRetry)).not.toContain('core:proposal-accept')
 expect(h.trace.slice(beforeRetry)).not.toContain('core:commit-object')
 expect(h.requests.filter(r=>r.command==='replaceCells'||r.command==='export'||r.command==='ack')).toHaveLength(0)
 expect(h.requests.filter(r=>r.command==='open')).toHaveLength(3)
 expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:edited,version:hash(edited)})
 expect(h.trace.filter(event=>event==='core:open-object')).toHaveLength(3)
 expect(h.getCommitCalls()).toBe(1);expect(h.getStatusCalls()).toBe(0);expect(h.getDestroyed()).toBe(2)
})
test('typed Agent committed readback rejects a later external revision before loading its bytes',async()=>{
 const h=setup(true);expect((await h.controller.request(h.openRequest)).ok).toBe(true);h.changeReadbackAfterCommit()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'conflict',view:{saving:false,status:'error',canUndo:false}})
 expect(h.getSaved()).toBe(hash(externallyEdited));expect(h.getPrepared()).toMatchObject({proposalId:h.proposalId,status:'committed',revision:hash(edited)})
 expect(h.requests.filter(r=>r.command==='open')).toEqual([expect.objectContaining({bytes:original,version:revision})])
 expect(h.requests.some(r=>r.command==='open'&&r.version===hash(externallyEdited))).toBe(false)
 const commit=h.trace.indexOf('core:commit-object'),readback=h.trace.indexOf('core:open-object',commit+1)
 expect(readback).toBeGreaterThan(commit)
 expect(h.trace.slice(readback+1)).toContain('surface:destroy')
 expect(h.trace.slice(readback+1)).not.toContain('surface:create')
 expect(h.trace.slice(commit+1)).not.toContain('view:ready')
 expect(h.requests.filter(r=>r.command==='replaceCells'||r.command==='export'||r.command==='ack')).toHaveLength(0)
 expect(h.getCommitCalls()).toBe(1);expect(h.getDestroyed()).toBe(1)
})
test('typed Agent commit rejection records its Core code while preserving save_failed',async()=>{
 const diagnostic=vi.spyOn(logger,'logNativeOfficeSaveFailure').mockImplementation(()=>undefined)
 try {
  const h=setup(true);h.rejectCommit()
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'save_failed',view:{dirty:false,saving:false,revision,recovery:{pending:{canCancel:true}}}})
  expect(diagnostic).toHaveBeenCalledExactlyOnceWith('save_failed',{phase:'commit',outerSchemaAccepted:true,innerSchemaAccepted:true,outerCode:null,innerCode:'not_text',receiptPresent:false,receiptStatus:null})
  expect(h.getSaved()).toBe(revision);expect(h.getCommitCalls()).toBe(1)
 } finally { diagnostic.mockRestore() }
})
test('typed Agent candidate generation failure preserves the readonly view and exposes durable approval cancellation',async()=>{
 const h=setup(true);expect((await h.controller.request({...open,path:'a.xlsx'})).ok).toBe(true);h.failTyped()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'unknown',view:{dirty:false,editing:true,saving:false,revision,recovery:{pending:{status:'prepared',canCancel:true}}}})
 expect(h.getDestroyed()).toBe(0)
 expect(h.requests.filter(r=>r.command==='open')).toHaveLength(1)
 await h.controller.saveAll()
 expect(h.requests.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
 expect(h.getCommitCalls()).toBe(0)
 expect(await h.controller.request({action:'cancelChange',threadId,changeId,...h.target()})).toMatchObject({ok:true,view:{dirty:false,revision}})
 expect(h.getCancelCalls()).toBe(1)
})

test.each(['missing','hash','length','binding'] as const)('typed Agent rejects a %s Core candidate before committing or native mutation',async(kind)=>{
 const h=setup(true)
 h.candidateOverride(candidate=>kind==='missing'?undefined:kind==='hash'?{...candidate,content:{...candidate.content,sha256:'0'.repeat(64)}}:
  kind==='length'?{...candidate,content:{...candidate.content,byteLength:edited.byteLength+1}}:{...candidate,changeId:'8'.repeat(64)})
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'invalid_response',view:{dirty:false,saving:false,revision}})
 expect(h.getCommitCalls()).toBe(0);expect(h.getSaved()).toBe(revision)
 expect(h.requests.some(r=>['replace','replaceCells','export','ack'].includes(r.command))).toBe(false)
})
test.each(['reply-lost','candidate-failed'] as const)('typed Agent retries the same decision after %s without another authority',async(kind)=>{
 const h=setup(true)
 if(kind==='reply-lost')h.loseDecision();else h.failTyped()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'unknown',view:{saving:false,dirty:false,recovery:{pending:{status:'prepared',canCancel:true}}}})
 h.failTyped(false)
 expect(await h.controller.request({action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId})).toMatchObject({ok:true,view:{canUndo:true,dirty:false}})
 expect(h.decisionInputs).toHaveLength(2);expect(h.decisionInputs[1]).toEqual(h.decisionInputs[0])
 expect(h.getCommitCalls()).toBe(1)
 expect(h.requests.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
})
const previousWorkbookChange=(proposalId='7'.repeat(48),undone=false)=>({changeId:'8'.repeat(64),threadId,proposalId,
 baseRevision:undone?revision:'6'.repeat(64),revision,status:undone?'undone':'committed',beforeText:'previous',afterText:'previous edit',
 saveOperationId:'save_previous_0001',undoOperationId:'undo_previous_0001',canUndo:false,canCancel:false,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''})
test.each(['no-current','old-current','old-undone-current'] as const)('typed Agent releases its rejected decision after conflict with %s so discard can close',async(kind)=>{
 const h=setup(true);h.refuseBeforeApproval()
 if(kind!=='no-current')h.seedRecoveryCurrent(previousWorkbookChange(undefined,kind==='old-undone-current'))
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'conflict',view:{revision,saving:false}})
 expect(h.getPrepared()).toBeUndefined();expect(h.controller.getView()?.recovery?.pending).toBeNull()
 expect(await h.controller.request({action:'close',objectId,discard:true})).toEqual({ok:true,view:null})
 expect(h.decisionInputs).toHaveLength(1);expect(h.getCommitCalls()).toBe(0);expect(h.getCancelCalls()).toBe(0)
})
test.each(['no-current','old-current','old-undone-current'] as const)('typed Agent can reference again after conflict with %s using a new decision operation',async(kind)=>{
 const h=setup(true);h.refuseBeforeApproval()
 if(kind!=='no-current')h.seedRecoveryCurrent(previousWorkbookChange(undefined,kind==='old-undone-current'))
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'conflict'})
 expect(await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:true})).toMatchObject({ok:true})
 expect(await h.controller.request({action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId})).toMatchObject({ok:true,view:{revision:hash(edited),canUndo:true}})
 expect(h.decisionInputs).toHaveLength(2);expect(h.decisionInputs[1].operationId).not.toBe(h.decisionInputs[0].operationId)
 expect(h.getCommitCalls()).toBe(1)
 expect(h.requests.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
})
test.each(['query-failed','reply-unknown','matching-current'] as const)('typed Agent retains its decision after %s rather than treating missing evidence as no approval',async(kind)=>{
 const h=setup(true);h.refuseBeforeApproval(kind==='reply-unknown'?'lost':'conflict')
 if(kind==='query-failed')h.failRecovery()
 if(kind==='matching-current')h.seedRecoveryCurrent(previousWorkbookChange(h.proposalId))
 const result=await applyProposal(h)
 expect(result).toMatchObject({ok:false,error:kind==='reply-unknown'?'unknown':'conflict',view:{revision,saving:false}})
 expect(await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:true})).toMatchObject({ok:false,error:'unsaved_changes'})
 expect(await h.controller.request({action:'close',objectId,discard:true})).toMatchObject({ok:false,error:'unknown'})
 expect(h.decisionInputs).toHaveLength(1);expect(h.getCommitCalls()).toBe(0)
})
test('typed Agent does not release a decision after its document owner is disposed during recovery',async()=>{
 const h=setup(true);h.refuseBeforeApproval()
 const decisionKey=`${h.scopeId}:${h.proposalId}:true`,deleted=vi.spyOn(Map.prototype,'delete')
 let shutdown:Promise<void>|undefined
 h.duringRecovery(()=>{shutdown=h.controller.destroy()})
 try{
  expect(await applyProposal(h)).toMatchObject({ok:false,error:'conflict'})
  expect(deleted.mock.calls.some(([key])=>key===decisionKey)).toBe(false)
  await shutdown
 }finally{deleted.mockRestore()}
 expect(h.decisionInputs).toHaveLength(1);expect(h.getCommitCalls()).toBe(0)
})
test('typed Agent rejects a changed native sequence while Core prepares the candidate',async()=>{
 const h=setup(true);h.duringAccept(()=>h.change())
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'stale_selection',view:{revision}})
 expect(h.getCommitCalls()).toBe(0);expect(h.getSaved()).toBe(revision)
 expect(h.requests.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
})
test('typed Agent refuses legacy text-only scopes without reinterpreting their approval',async()=>{
 const h=setup(true);h.scopeOverride({workbook:undefined})
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'unsupported_selection'})
 expect(h.decisionInputs).toHaveLength(0);expect(h.getCommitCalls()).toBe(0)
 expect(h.requests.some(r=>['replace','replaceCells','export','ack'].includes(r.command))).toBe(false)
})
test('typed Agent commit unknown only queries and resumes its retained candidate without accepting again',async()=>{
 const h=setup(true);h.interruptSave()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'unknown',view:{saving:true,dirty:false}})
 const decision={action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId}
 expect(await h.controller.request(decision)).toMatchObject({ok:false,error:'unsaved_changes'})
 expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'unknown',view:{recovery:{pending:{canResume:true}}}})
 expect(h.decisionInputs).toHaveLength(1);expect(h.getCommitCalls()).toBe(1);expect(h.getStatusCalls()).toBe(1)
 expect(await h.controller.request({action:'resumeChange',threadId,changeId,...h.target()})).toMatchObject({ok:true,view:{revision:hash(edited),dirty:false,editing:false,canUndo:true}})
 expect(h.decisionInputs).toHaveLength(1);expect(h.getCommitCalls()).toBe(1);expect(h.getResumeCalls()).toBe(1)
 expect(h.requests.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
})
test('typed Agent lost committed reply is resolved by status and its readonly reopened file can be undone',async()=>{
 const h=setup(true);h.loseCommit()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'unknown',view:{saving:true,dirty:false}})
 expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{revision:hash(edited),canUndo:true,dirty:false,editing:false}})
 expect(h.decisionInputs).toHaveLength(1);expect(h.getCommitCalls()).toBe(1);expect(h.getStatusCalls()).toBe(1)
 expect(await h.controller.request({action:'undoChange',threadId,...h.target()})).toMatchObject({ok:true,view:{revision,canUndo:false,dirty:false,editing:false}})
 expect(h.getUndoCalls()).toBe(1);expect(h.getSaved()).toBe(revision)
 expect(h.requests.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
})
test('typed Agent known commit refusal permits cancellation without rebuilding or retrying the commit',async()=>{
 const h=setup(true);h.rejectCommit()
 expect(await applyProposal(h)).toMatchObject({ok:false,error:'save_failed',view:{saving:false,dirty:false}})
 expect(await h.controller.request({action:'acceptProposal',...h.target(),scopeId:h.scopeId,proposalId:h.proposalId})).toMatchObject({ok:false,error:'unsaved_changes'})
 expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{recovery:{pending:{canCancel:true}}}})
 expect(h.decisionInputs).toHaveLength(1);expect(h.getCommitCalls()).toBe(1)
 expect(await h.controller.request({action:'cancelChange',threadId,changeId,...h.target()})).toMatchObject({ok:true,view:{revision,dirty:false}})
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


test('readonly reference preparation and challenge use Core without requesting native edits',async()=>{
  const h=setup();await h.controller.request(open)
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:false})).toMatchObject({ok:true,view:{scope:{scopeId:h.scopeId,editable:false}}})
  expect(await h.controller.request({action:'proposals',objectId,scopeId:h.scopeId})).toMatchObject({ok:true})
  expect(h.controller.getView()?.editing).toBeFalsy()
  expect(h.selectionCalls).toEqual(['capture-selection','selection-read'])
  expect(h.requests.every(request=>['open','captureSelection'].includes(request.command))).toBe(true)
  expect(h.getCommitCalls()).toBe(0)
  h.refuseSelectionRead()
  expect(await h.controller.request({action:'proposals',objectId,scopeId:h.scopeId})).toMatchObject({ok:false,error:'stale_selection'})
})
test.each([
  {sessionId:'f'.repeat(48)},{threadId:'other'},{selectionToken:'other_token'},
  {baseRevision:'f'.repeat(64)},{changeSequence:1},{editable:true}
])('rejects a schema-valid but wrongly bound readonly capture %j',async mismatch=>{
  const h=setup();await h.controller.request(open);h.scopeOverride(mismatch)
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:false})).toMatchObject({ok:false,error:'invalid_response'})
  expect(h.controller.getView()?.scope).toBeUndefined()
  expect(h.getCommitCalls()).toBe(0)
})
test('rejects a readonly Core challenge whose binding drifted since capture',async()=>{
  const h=setup();await h.controller.request(open)
  await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:false})
  h.scopeOverride({baseRevision:'f'.repeat(64)})
  expect(await h.controller.request({action:'proposals',objectId,scopeId:h.scopeId})).toMatchObject({ok:false,error:'stale_selection'})
  expect(h.getCommitCalls()).toBe(0)
})


test('dirty selection references require explicit save-first before capturing Core authority',async()=>{
  const h=setup();await h.controller.request(open);await h.controller.request({action:'annotate',...h.target()});h.change()
  const count=h.selectionCalls.filter(call=>call==='capture-selection').length
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:'selection_123',threadId,editable:false})).toMatchObject({ok:false,error:'unsaved_changes',view:{dirty:true}})
  expect(h.selectionCalls.filter(call=>call==='capture-selection')).toHaveLength(count)
  expect(h.getCommitCalls()).toBe(0)
})

// Synthetic protocol fixture only: a freeze ACK below is not Qt/IME runtime evidence.
function setupManual(beginResponse?: unknown) {
  type ManualAuthority = {version:2;origin:'manual';changeId:string;baseRevision:string;saveOperationId:string;undoOperationId:string}
  type ManualState = {version:2;origin:'manual';changeId:string;baseRevision:string;generation:number;frozen:boolean}
  type State = {documentId:string;version:string;kind:'docx';changeSequence:number;acknowledgedSequence:number;dirty:boolean;manual?:ManualState}
  let current:State = {documentId:objectId,version:revision,kind:'docx',changeSequence:0,acknowledgedSequence:0,dirty:false}
  let savedBytes=original.slice(), workBytes=original.slice(), opened='', generation=0, serial=0, operation=0
  let onEvent:(event:unknown)=>void=()=>{}, pending:any, committed:any, activeAuthority:ManualAuthority|undefined
  let loseBegin=false, invalidAuthority=false, loseCommit=false, refuseFreeze=false, emptyCapture=false, foreignFreeze=false, wrongExportSequence=false
  let queuedInput=false, compositionPending=false, refuseCommit=false, refuseExport=false, loseCancelReply=false
  let loseAck:'before'|'after'|undefined
  let lastLease:{generation:number;sequence:number;token:string}|undefined
  let formatState={fontFamily:'Noto Sans CJK SC',fontSize:12,bold:false,italic:false,underline:false}
  const requests:NativeOfficeEngineRequest[]=[], calls:{operation:string;input:any}[]=[], order:string[]=[]
  const begins=new Map<string,ManualAuthority>(), receipts=new Map<string,any>(), cancelled=new Map<string,any>()
  const pkg:PluginPackageView={packageId:'analytix-documents',packageVersion:'1.0.0',displayName:'Documents',origin:'development-source',publishable:false,materialized:true,generationId:'b'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'c'.repeat(64),available:true,operations:['open-object','close-object','begin-manual-edit','commit-object','object-status','capture-selection','selection-revoke','object-recovery','cancel-change','undo-change']}
  const selection=(empty=false)=>{
    const token=`manual_selection_${++serial}`
    lastLease=current.manual?{generation:current.manual.generation,sequence:current.changeSequence,token}:undefined
    const text=empty?'':'selected'
    return {documentId:objectId,version:current.version,changeSequence:current.changeSequence,kind:'text',scope:'session-text-range-at-version-and-change-sequence',text,token,
      capture:{capturedCharacters:text.length,totalCharacters:text.length,truncated:false,unit:'utf-16',complete:true},
      ...(lastLease?{rangeLease:{...lastLease},formatState:{...formatState}}:{})}
  }
  const mutate=()=>{
    expect(current.manual?.frozen).toBe(false)
    current={...current,changeSequence:current.changeSequence+1,dirty:true}
    workBytes=new Uint8Array([80,75,workBytes[2]+1])
  }
  const controller=createNativeOfficeController({
    newOperationId:()=>`manual_host_${++operation}`,
    onChange(){},
    async packageHost(request):Promise<any>{
      if(request.action==='list')return {ok:true,packages:[pkg]}
      if(request.action!=='invoke')throw Error('unexpected package action')
      const {operation:op,input}=request
      calls.push({operation:op,input:structuredClone(input)});order.push(`core:${op}`)
      if(op==='open-object')return {ok:true,output:{ok:true,document:{objectId,sessionId,path:'/workspace/a.docx',revision:hash(savedBytes),content:Buffer.from(savedBytes).toString('base64')}}}
      if(op==='object-recovery')return {ok:true,output:{ok:true,recovery:{current:committed??null,pending:pending??null}}}
      if(op==='begin-manual-edit'){
        expect(input).toMatchObject({sessionId,threadId,baseRevision:hash(savedBytes)})
        if(beginResponse !== undefined)return beginResponse
        const id=input.operationId as string
        let authority=begins.get(id)
        if(!authority){
          const index=begins.size+1
          authority={version:2,origin:'manual',changeId:hash(Buffer.from(`manual-change-${index}`)),baseRevision:hash(savedBytes),saveOperationId:`manual_save_${index}`,undoOperationId:`manual_undo_${index}`}
          begins.set(id,authority)
          pending={...authority,threadId,revision:'',status:'prepared',canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'2026-10-15T00:00:00Z',savedAt:''}
        }
        if(loseBegin){loseBegin=false;throw Error('reply lost after reserving manual authority')}
        if(invalidAuthority){const {version:_version,origin:_origin,...missingOrigin}=authority;return {ok:true,output:{ok:true,manual:missingOrigin}}}
        return {ok:true,output:{ok:true,manual:authority}}
      }
      if(op==='commit-object'){
        expect(input).toMatchObject({sessionId,threadId,changeId:pending.changeId,baseRevision:pending.baseRevision,operationId:pending.saveOperationId})
        if(refuseCommit){refuseCommit=false;return {ok:true,output:{ok:false,code:'not_text',message:'synthetic candidate refusal'}}}
        const bytes=new Uint8Array(Buffer.from((input.content as any).data,'base64'))
        expect(input.content).toEqual({encoding:'base64',kind:'docx',data:Buffer.from(bytes).toString('base64'),sha256:hash(bytes),byteLength:bytes.byteLength})
        expect(input.baseRevision).toBe(hash(savedBytes));savedBytes=bytes
        const receipt={operationId:input.operationId,revision:hash(bytes),status:'committed',savedAt:'2026-10-15T00:01:00Z'}
        receipts.set(input.operationId as string,receipt)
        committed={...pending,revision:receipt.revision,status:'committed',savedAt:receipt.savedAt,canUndo:true,canCancel:false};pending=undefined
        if(loseCommit){loseCommit=false;throw Error('reply lost after committing bytes')}
        return {ok:true,output:{ok:true,receipt}}
      }
      if(op==='object-status')return {ok:true,output:{ok:true,receipt:receipts.get(input.operationId as string)}}
      if(op==='cancel-change'){
        const prior=pending??cancelled.get(input.changeId as string)
        expect(prior).toMatchObject({version:2,origin:'manual',revision:'',canCancel:true})
        expect(input).toEqual({sessionId,threadId,changeId:prior.changeId,baseRevision:hash(savedBytes)})
        if(pending){cancelled.set(pending.changeId,structuredClone(pending));pending=undefined}
        else expect(cancelled.has(input.changeId as string)).toBe(true)
        if(loseCancelReply){loseCancelReply=false;throw Error('reply lost after Core cancelled the manual reservation')}
        return {ok:true,output:{ok:true,recovery:{current:committed??null,pending:null}}}
      }
      if(op==='capture-selection'){
        const {text,...binding}=input
        expect(text).toBe('selected')
        return {ok:true,output:{ok:true,scope:{...binding,scopeId:'d'.repeat(48),parts:[{kind:'literal',text}]}}}
      }
      if(op==='close-object')return {ok:true,output:{ok:true,closed:true}}
      if(op==='selection-revoke')return {ok:true,output:{ok:true,revoked:true}}
      throw Error(`unexpected Core operation: ${op}`)
    },
    createSurface(listener){onEvent=listener;return {attach(){},hide(){order.push('surface:hide')},destroy(){order.push('surface:destroy')},async request(request){
      requests.push(structuredClone(request));order.push(`engine:${request.command}`)
      const base={type:'result',channel:'surface',command:request.command,operationId:request.operationId,documentId:request.documentId,version:request.version,ok:true}
      if(request.command==='close')return base
      if(request.command==='open'){
        opened=request.operationId;workBytes=request.bytes!.slice();activeAuthority=undefined
        current={documentId:objectId,version:request.version,kind:'docx',changeSequence:0,acknowledgedSequence:0,dirty:false}
      }
      if(request.command==='manual-edit'){
        expect([...begins.values()]).toContainEqual(request.manual)
        activeAuthority=request.manual!
        const {saveOperationId:_save,undoOperationId:_undo,...identity}=activeAuthority
        current={...current,manual:{...identity,generation:++generation,frozen:false}}
      }
      if(request.command==='format'){
        expect(request.changeId).toBe(activeAuthority?.changeId)
        if(JSON.stringify(request.rangeLease)!==JSON.stringify(lastLease))throw Error('stale-selection')
        mutate()
        const format=request.format!
        if(format.kind==='font-family')formatState={...formatState,fontFamily:format.value}
        else if(format.kind==='font-size')formatState={...formatState,fontSize:format.value}
        else formatState={...formatState,[format.kind]:format.value}
        const selected=selection()
        return {...base,state:structuredClone(current),selection:selected,rangeLease:{...lastLease!},formatState:{...formatState}}
      }
      if(request.command==='validateSelection'){
        if(request.selectionToken!==lastLease?.token||request.expectedChangeSequence!==current.changeSequence)throw Error('stale-selection')
        return {...base,state:structuredClone(current)}
      }
      if(request.command==='freeze'){
        if(refuseFreeze)throw Error('native-input-unavailable')
        if(compositionPending)throw Error('native-composition-pending')
        if(queuedInput){queuedInput=false;mutate()}
        expect(request.changeId).toBe(activeAuthority?.changeId)
        current={...current,manual:{...current.manual!,frozen:true}}
        return {...base,state:structuredClone(current),freezeAck:{changeId:activeAuthority!.changeId,operationId:foreignFreeze?'stale_freeze_0001':request.operationId,generation:current.manual!.generation,sequence:current.changeSequence}}
      }
      if(request.command==='export'){
        const frozen=requests.filter(r=>r.command==='freeze').at(-1)!
        expect(request.freezeAck).toEqual({changeId:activeAuthority!.changeId,operationId:frozen.operationId,generation:current.manual!.generation,sequence:current.changeSequence})
        expect(request).toMatchObject({changeId:activeAuthority!.changeId,operationId:activeAuthority!.saveOperationId})
        if(refuseExport){refuseExport=false;throw Error('native-input-unavailable')}
        return {...base,state:structuredClone(current),bytes:workBytes.slice(),exportedSequence:current.changeSequence-(wrongExportSequence?1:0)}
      }
      if(request.command==='ack'){
        if(request.status==='failed'){
          expect(request).toMatchObject({exportOperationId:activeAuthority!.saveOperationId,exportedSequence:current.changeSequence})
          return {...base,state:structuredClone(current),selection:selection()}
        }
        expect(request).toMatchObject({status:'committed',exportOperationId:activeAuthority!.saveOperationId,exportedSequence:current.changeSequence,persistedVersion:hash(savedBytes)})
        const failure=loseAck;loseAck=undefined
        if(failure==='before')throw Error('native-input-unavailable')
        const {manual:_manual,...state}=current
        current={...state,version:request.persistedVersion!,acknowledgedSequence:request.exportedSequence!,dirty:false}
        activeAuthority=undefined
        if(failure==='after')throw Error('office-operation-timeout-unknown')
      }
      const empty=request.command==='captureSelection'&&emptyCapture
      if(empty)emptyCapture=false
      return {...base,state:structuredClone(current),selection:selection(empty)}
    }}}
  })
  const target=()=>({objectId,revision:current.version,expectedChangeSequence:current.changeSequence})
  return {controller,requests,calls,order,target,
    begin:()=>controller.request({action:'beginManualEdit',threadId,...target()}),
    change(){mutate();onEvent({type:'changed',operationId:opened,documentId:objectId,version:current.version,state:structuredClone(current)})},
    lease:()=>({...lastLease!}),driftSelection:()=>{lastLease={...lastLease!,token:'worker_range_moved'}},authority:()=>[...begins.values()].at(-1)!,saved:()=>hash(savedBytes),
    loseBegin:()=>{loseBegin=true},invalidAuthority:()=>{invalidAuthority=true},loseCommit:()=>{loseCommit=true},loseAck:(when:'before'|'after')=>{loseAck=when},
    queuedInput:()=>{queuedInput=true},compositionPending:()=>{compositionPending=true},refuseCommit:()=>{refuseCommit=true},refuseExport:()=>{refuseExport=true},loseCancelReply:()=>{loseCancelReply=true},
    refuseFreeze:()=>{refuseFreeze=true},emptyCapture:()=>{emptyCapture=true},foreignFreeze:()=>{foreignFreeze=true},wrongExportSequence:()=>{wrongExportSequence=true}}
}

const manualBeginPrivateDetail='SYNTHETIC_PRIVATE_MANUAL_BEGIN_DETAIL_31E7'
test.each([
  {name:'validated inner refusal',reply:{ok:true,output:{ok:false,code:'not_text',message:manualBeginPrivateDetail}},error:'unknown',folded:'unavailable',state:{schemaAccepted:true,outerCode:null,innerCode:'not_text'}},
  {name:'validated inner forbidden',reply:{ok:true,output:{ok:false,code:'forbidden',message:manualBeginPrivateDetail}},error:'forbidden',folded:'forbidden',state:{schemaAccepted:true,outerCode:null,innerCode:'forbidden'}},
  {name:'validated outer refusal',reply:{ok:false,code:'conflict',message:manualBeginPrivateDetail},error:'package_unavailable',folded:'package_unavailable',state:{schemaAccepted:true,outerCode:'conflict',innerCode:null}},
  {name:'unvalidated outer code',reply:{ok:false,code:manualBeginPrivateDetail,message:manualBeginPrivateDetail},error:'invalid_response',folded:'invalid_response',state:{schemaAccepted:false,outerCode:null,innerCode:null}},
  {name:'unvalidated inner code',reply:{ok:true,output:{ok:false,code:manualBeginPrivateDetail,message:manualBeginPrivateDetail}},error:'invalid_response',folded:'invalid_response',state:{schemaAccepted:false,outerCode:null,innerCode:null}}
])('manual begin diagnostics preserve only the closed cause of $name',async({reply,error,folded,state})=>{
  const diagnostic=vi.spyOn(logger,'logNativeOfficeManualBeginFailure').mockImplementation(()=>undefined)
  try {
    const h=setupManual(reply);await h.controller.request(open)
    expect(await h.begin()).toMatchObject({ok:false,error,view:{dirty:false}})
    expect(diagnostic).toHaveBeenCalledExactlyOnceWith(folded,{...state,pendingSave:false,pendingRecovery:false,pendingManualCancel:false})
    expect(JSON.stringify(diagnostic.mock.calls)).not.toContain(manualBeginPrivateDetail)
    expect(h.calls.filter(call=>call.operation==='begin-manual-edit')).toHaveLength(1)
    expect(h.requests.some(request=>['manual-edit','edit','format','export'].includes(request.command))).toBe(false)
    expect(h.controller.getView()?.manual).toBeUndefined()
  } finally {diagnostic.mockRestore()}
})

test('manual input waits for explicit Core authority and retries the same uncertain begin operation',async()=>{
  const diagnostic=vi.spyOn(logger,'logNativeOfficeManualBeginFailure').mockImplementation(()=>undefined)
  try {
    const h=setupManual();await h.controller.request(open);h.loseBegin()
    expect(await h.begin()).toMatchObject({ok:false,error:'unknown',view:{dirty:false}})
    expect(diagnostic).toHaveBeenCalledExactlyOnceWith('unavailable',{schemaAccepted:null,outerCode:null,innerCode:null,pendingSave:false,pendingRecovery:false,pendingManualCancel:false})
    expect(h.requests.some(r=>r.command==='manual-edit')).toBe(false)
    expect(await h.controller.request({...open,threadId:'other-thread'})).toMatchObject({ok:false,error:'unknown'})
    expect(h.calls.some(c=>c.operation==='begin-manual-edit'&&c.input.threadId==='other-thread')).toBe(false)
    expect(await h.begin()).toMatchObject({ok:true,view:{editing:true,manual:{version:2,origin:'manual',frozen:false}}})
    const begins=h.calls.filter(c=>c.operation==='begin-manual-edit')
    expect(begins).toHaveLength(2);expect(begins[1].input).toEqual(begins[0].input)
    expect(h.order.indexOf('core:begin-manual-edit')).toBeLessThan(h.order.indexOf('engine:manual-edit'))
    const count=h.requests.length
    expect(await h.begin()).toMatchObject({ok:true})
    expect(h.calls.filter(c=>c.operation==='begin-manual-edit')).toHaveLength(2);expect(h.requests).toHaveLength(count)
    expect(diagnostic).toHaveBeenCalledOnce()
  } finally {diagnostic.mockRestore()}
})
test('a clean manual authority cannot transfer to another thread and closing cancels its reservation without reopening',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin()
  const count=h.requests.length
  expect(await h.controller.request({...open,threadId:'other-thread'})).toMatchObject({ok:false,error:'unsaved_changes',view:{dirty:false,manual:{origin:'manual'}}})
  expect(h.requests).toHaveLength(count)
  expect(h.calls.some(c=>c.operation==='begin-manual-edit'&&c.input.threadId==='other-thread')).toBe(false)
  expect(await h.begin()).toMatchObject({ok:true})
  expect(await h.controller.request({action:'close',objectId})).toEqual({ok:true,view:null})
  expect(h.calls.filter(c=>c.operation==='cancel-change')).toHaveLength(1)
  expect(h.requests.filter(r=>r.command==='open')).toHaveLength(1)
  expect(h.calls.some(c=>c.operation==='commit-object'||c.operation==='undo-change')).toBe(false)
  expect(h.saved()).toBe(revision)
})
test('closing a clean manual view freezes queued input and the later save reuses that exact proof once',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.queuedInput()
  expect(h.controller.getView()).toMatchObject({dirty:false,changeSequence:0})
  expect(await h.controller.request({action:'close',objectId})).toMatchObject({ok:false,error:'unsaved_changes',view:{revision,dirty:true,changeSequence:1,manual:{frozen:true}}})
  const freeze=h.requests.filter(r=>r.command==='freeze')
  expect(freeze).toHaveLength(1)
  expect(h.requests.some(r=>r.command==='close'||r.command==='export')).toBe(false)
  expect(h.calls.some(c=>c.operation==='cancel-change'||c.operation==='close-object'||c.operation==='commit-object')).toBe(false)
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:true,view:{revision:h.saved(),dirty:false,editing:false}})
  expect(h.requests.filter(r=>r.command==='freeze')).toEqual(freeze)
  expect(h.requests.filter(r=>r.command==='export')).toHaveLength(1)
  expect(h.requests.find(r=>r.command==='export')?.freezeAck?.operationId).toBe(freeze[0].operationId)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
  expect(h.saved()).toBe(hash(edited))
})
test('pending composition prevents a clean manual close from cancelling its Core reservation',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.compositionPending()
  expect(await h.controller.request({action:'close',objectId})).toMatchObject({ok:false,error:'composition_pending',view:{revision,dirty:false,manual:{origin:'manual'}}})
  expect(h.requests.filter(r=>r.command==='freeze')).toHaveLength(1)
  expect(h.requests.some(r=>r.command==='close'||r.command==='export'||r.command==='ack')).toBe(false)
  expect(h.calls.some(c=>c.operation==='cancel-change'||c.operation==='close-object'||c.operation==='commit-object')).toBe(false)
  expect(h.saved()).toBe(revision)
})
test('missing proposal fields never substitute for the explicit manual authority discriminator',async()=>{
  const h=setupManual();await h.controller.request(open);h.invalidAuthority()
  expect(await h.begin()).toMatchObject({ok:false,error:'invalid_response',view:{dirty:false}})
  expect(h.controller.getView()?.manual).toBeUndefined()
  expect(h.requests.some(r=>r.command==='manual-edit'||r.command==='format'||r.command==='export')).toBe(false)
})
test('manual formatting rejects a forged or stale lease and accepts each renewed native token',async()=>{
  const h=setupManual();await h.controller.request(open);expect((await h.begin()).ok).toBe(true)
  const lease=h.lease(),changeId=h.authority().changeId
  expect(await h.controller.request({action:'format',...h.target(),changeId,rangeLease:{...lease,token:'forged_selection'},format:{kind:'bold',value:true}})).toMatchObject({ok:false,error:'stale_selection'})
  expect(await h.controller.request({action:'format',...h.target(),changeId,rangeLease:lease,format:{kind:'bold',value:true}})).toMatchObject({ok:true,view:{dirty:true,changeSequence:1,selection:{formatState:{bold:true}}}})
  const renewed=h.lease();expect(renewed.token).not.toBe(lease.token)
  expect(await h.controller.request({action:'format',...h.target(),changeId,rangeLease:lease,format:{kind:'italic',value:true}})).toMatchObject({ok:false,error:'stale_selection'})
  expect(await h.controller.request({action:'format',...h.target(),changeId,rangeLease:renewed,format:{kind:'font-family',value:'Noto Serif CJK SC'}})).toMatchObject({ok:true,view:{changeSequence:2,selection:{formatState:{bold:true,fontFamily:'Noto Serif CJK SC'}}}})
  expect(h.requests.filter(r=>r.command==='format')).toHaveLength(3)
  expect(h.calls.some(c=>c.operation==='commit-object')).toBe(false)
})
test('saveAll freezes queued native input in a clean manual view before deciding whether Core has bytes to commit',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.queuedInput()
  expect(h.controller.getView()).toMatchObject({dirty:false,changeSequence:0})
  expect(await h.controller.saveAll()).toMatchObject({ok:true,view:{revision:h.saved(),dirty:false,editing:false,changeSequence:1,acknowledgedSequence:1}})
  expect(h.saved()).toBe(hash(edited))
  expect(h.requests.filter(r=>r.command==='freeze')).toHaveLength(1)
  expect(h.requests.filter(r=>r.command==='export')).toHaveLength(1)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
  expect(h.calls.some(c=>c.operation==='cancel-change')).toBe(false)
})
test('a clean manual view with pending composition cannot save or export before native confirmation',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.compositionPending()
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'composition_pending',view:{dirty:false,revision}})
  expect(h.requests.filter(r=>r.command==='freeze')).toHaveLength(1)
  expect(h.requests.some(r=>r.command==='export'||r.command==='ack')).toBe(false)
  expect(h.calls.some(c=>c.operation==='commit-object'||c.operation==='cancel-change')).toBe(false)
  expect(h.saved()).toBe(revision)
})
test('a truly clean native freeze cancels the reservation and rereads Core into a readonly copy without a commit',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin()
  const count=h.order.length
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:true,view:{revision,dirty:false}})
  expect(h.controller.getView()?.manual).toBeUndefined();expect(h.controller.getView()?.editing).not.toBe(true)
  expect(h.requests.filter(r=>r.command==='freeze')).toHaveLength(1)
  expect(h.requests.some(r=>r.command==='export'||r.command==='ack')).toBe(false)
  expect(h.calls.filter(c=>c.operation==='cancel-change')).toHaveLength(1)
  expect(h.calls.some(c=>c.operation==='commit-object'||c.operation==='undo-change')).toBe(false)
  expect(h.order.slice(count).filter(item=>['engine:freeze','core:open-object','core:cancel-change','engine:open'].includes(item))).toEqual(['engine:freeze','core:open-object','core:cancel-change','core:open-object','engine:open'])
  expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:original,version:revision})
})
test('a rejected frozen manual save requires explicit recovery before another begin or save can mutate',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.change();h.refuseCommit()
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'save_failed',view:{dirty:true,saving:false,manual:{frozen:true}}})
  const count=h.requests.length
  expect(await h.begin()).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(h.requests).toHaveLength(count)
  expect(h.calls.filter(c=>c.operation==='begin-manual-edit')).toHaveLength(1)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
  expect(h.calls.some(c=>c.operation==='object-status')).toBe(false)
  expect(h.saved()).toBe(revision)
})
test('a failed manual export consumes the attempt and cannot freeze or export its working copy again',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.change();h.refuseExport()
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'native_input_unavailable',view:{dirty:true,manual:{frozen:true}}})
  const count=h.requests.length
  expect(await h.begin()).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(h.requests).toHaveLength(count)
  expect(h.requests.filter(r=>r.command==='export')).toHaveLength(1)
  expect(h.calls.some(c=>c.operation==='commit-object'||c.operation==='object-status')).toBe(false)
  expect(h.saved()).toBe(revision)
})
test('manual saves bind freeze, export and receipt ACK, then require a new transaction at the saved revision',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.change()
  const first=h.authority()
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:true,view:{dirty:false,editing:false,canUndo:true}})
  const firstRevision=h.saved(),view=h.controller.getView()
  expect(view?.manual).toBeUndefined();expect(view?.revision).toBe(firstRevision)
  expect(h.order.filter(item=>['engine:freeze','engine:export','core:commit-object','engine:ack'].includes(item))).toEqual(['engine:freeze','engine:export','core:commit-object','engine:ack'])
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect((await h.begin()).ok).toBe(true)
  const next=h.authority()
  expect(next.baseRevision).toBe(firstRevision);expect(next.changeId).not.toBe(first.changeId);expect(next.saveOperationId).not.toBe(first.saveOperationId)
  h.change();expect((await h.controller.request({action:'save',...h.target()})).ok).toBe(true)
  const commits=h.calls.filter(c=>c.operation==='commit-object')
  expect(commits.map(c=>[c.input.changeId,c.input.operationId,c.input.baseRevision])).toEqual([[first.changeId,first.saveOperationId,revision],[next.changeId,next.saveOperationId,firstRevision]])
  expect(h.saved()).not.toBe(firstRevision)
})
test.each(['foreign freeze','different export sequence'] as const)('manual save rejects %s before Core can write',async(kind)=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.change()
  if(kind==='foreign freeze')h.foreignFreeze();else h.wrongExportSequence()
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'invalid_response',view:{dirty:true,revision}})
  expect(h.calls.some(c=>c.operation==='commit-object')).toBe(false)
  expect(h.requests.some(r=>r.command==='ack')).toBe(false)
  if(kind==='foreign freeze')expect(h.requests.some(r=>r.command==='export')).toBe(false)
})
test('an unknown manual save only queries its original operation and never exports or commits twice',async()=>{
  const diagnostic=vi.spyOn(logger,'logNativeOfficeManualBeginFailure').mockImplementation(()=>undefined)
  try {
    const h=setupManual();await h.controller.request(open);await h.begin();h.change();h.loseCommit()
    expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:false,error:'unknown',view:{dirty:true,saving:true,manual:{frozen:true}}})
    const count=h.requests.length, authority=h.authority()
    expect(await h.begin()).toMatchObject({ok:false,error:'unknown'})
    expect(diagnostic).toHaveBeenCalledExactlyOnceWith('unknown',{schemaAccepted:null,outerCode:null,innerCode:null,pendingSave:true,pendingRecovery:false,pendingManualCancel:false})
    expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:true,view:{dirty:false,saving:false,editing:false}})
    expect(h.calls.filter(c=>c.operation==='object-status').map(c=>c.input.operationId)).toEqual([authority.saveOperationId])
    expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
    expect(h.requests.slice(count).map(r=>r.command)).toEqual(['ack'])
  } finally {diagnostic.mockRestore()}
})
test.each(['before','after'] as const)('a confirmed manual commit recovers when the native ACK fails %s applying without a second commit',async(when)=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.change();h.loseAck(when)
  const count=h.order.length
  expect(await h.controller.request({action:'save',...h.target()})).toMatchObject({ok:true,view:{revision:h.saved(),dirty:false,editing:false,saving:false,canUndo:true,changeSequence:0,acknowledgedSequence:0}})
  expect(h.controller.getView()?.manual).toBeUndefined()
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
  expect(h.calls.some(c=>c.operation==='object-status'||c.operation==='undo-change')).toBe(false)
  expect(h.requests.filter(r=>r.command==='ack')).toHaveLength(1)
  expect(h.requests.filter(r=>r.command==='export')).toHaveLength(1)
  expect(h.requests.filter(r=>r.command==='open')).toHaveLength(2)
  expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({version:h.saved(),bytes:edited})
  expect(h.order.slice(count).filter(item=>['core:commit-object','engine:ack','core:open-object','engine:open'].includes(item))).toEqual(['core:commit-object','engine:ack','core:open-object','engine:open'])
  expect((await h.controller.request({action:'saveStatus',objectId})).ok).toBe(true)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
})
test('discard cancels only the prepared manual copy and reopens Core bytes without undoing or deleting the file',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.change()
  const count=h.order.length,changeId=h.authority().changeId
  expect(await h.controller.request({action:'discardManualEdit',threadId,changeId,...h.target()})).toMatchObject({ok:true,view:{revision,dirty:false}})
  expect(h.controller.getView()?.editing).not.toBe(true)
  expect(h.saved()).toBe(revision)
  expect(h.calls.filter(c=>c.operation==='cancel-change')).toHaveLength(1)
  expect(h.calls.some(c=>c.operation==='commit-object'||c.operation==='undo-change')).toBe(false)
  expect(h.order.slice(count).filter(item=>['core:open-object','core:cancel-change','engine:open'].includes(item))).toEqual(['core:open-object','core:cancel-change','core:open-object','engine:open'])
  expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:original,version:revision})
  expect(h.controller.getView()?.manual).toBeUndefined()
  expect(await h.controller.request({action:'discardManualEdit',threadId,changeId,...h.target()})).toMatchObject({ok:false,error:'invalid_request'})
  expect(h.calls.filter(c=>c.operation==='cancel-change')).toHaveLength(1)
})
test('a lost discard reply retries the same cancelled change and blocks input, formatting and references until confirmed',async()=>{
  const diagnostic=vi.spyOn(logger,'logNativeOfficeManualBeginFailure').mockImplementation(()=>undefined)
  try {
    const h=setupManual();await h.controller.request(open);await h.begin();h.change();await h.controller.request({action:'captureSelection'})
    const changeId=h.authority().changeId,rangeLease=h.lease()
    h.loseCancelReply()
    expect(await h.controller.request({action:'discardManualEdit',threadId,changeId,...h.target()})).toMatchObject({ok:false,error:'unknown'})
    const count=h.requests.length
    expect(await h.begin()).toMatchObject({ok:false,error:'unknown'})
    expect(diagnostic).toHaveBeenCalledExactlyOnceWith('unknown',{schemaAccepted:null,outerCode:null,innerCode:null,pendingSave:false,pendingRecovery:false,pendingManualCancel:true})
    expect(await h.controller.request({action:'format',...h.target(),changeId,rangeLease,format:{kind:'bold',value:true}})).toMatchObject({ok:false,error:'unknown'})
    expect(await h.controller.request({action:'reference',...h.target(),selectionToken:rangeLease.token,threadId,editable:false,saveFirst:true})).toMatchObject({ok:false,error:'unknown'})
    expect(h.requests).toHaveLength(count)
    expect(h.calls.filter(c=>c.operation==='cancel-change')).toHaveLength(1)
    const recoveryReads=h.calls.filter(c=>c.operation==='object-recovery').length
    expect(await h.controller.request({action:'saveStatus',objectId})).toMatchObject({ok:false,error:'unknown'})
    expect(h.calls.filter(c=>c.operation==='object-recovery')).toHaveLength(recoveryReads+1)
    expect(h.calls.filter(c=>c.operation==='cancel-change')).toHaveLength(1)
    expect(h.requests).toHaveLength(count)
    expect(await h.controller.request({action:'discardManualEdit',threadId,changeId,...h.target()})).toMatchObject({ok:true,view:{revision,dirty:false}})
    expect(h.controller.getView()?.manual).toBeUndefined();expect(h.controller.getView()?.editing).not.toBe(true)
    const cancels=h.calls.filter(c=>c.operation==='cancel-change')
    expect(cancels).toHaveLength(2);expect(cancels[1].input).toEqual(cancels[0].input)
    expect(cancels[0].input).toEqual({sessionId,threadId,changeId,baseRevision:revision})
    expect(h.order.indexOf('surface:destroy')).toBeGreaterThan(-1)
    expect(h.order.indexOf('surface:destroy')).toBeLessThan(h.order.indexOf('core:cancel-change'))
    expect(h.calls.some(c=>c.operation==='commit-object'||c.operation==='undo-change'||c.operation==='capture-selection')).toBe(false)
    expect(h.requests.filter(r=>r.command==='open').at(-1)).toMatchObject({bytes:original,version:revision})
    expect(h.saved()).toBe(revision)
  } finally {diagnostic.mockRestore()}
})
test('an unreported native range movement rejects a clean manual reference before Core captures stale text',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin()
  const token=h.lease().token,count=h.requests.length
  h.driftSelection()
  expect(h.controller.getView()).toMatchObject({dirty:false,changeSequence:0})
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:token,threadId,editable:false})).toMatchObject({ok:false,error:'stale_selection'})
  expect(h.requests.slice(count).map(r=>r.command)).toEqual(['validateSelection'])
  expect(h.calls.some(c=>c.operation==='capture-selection'||c.operation==='commit-object')).toBe(false)
  expect(h.saved()).toBe(revision)
})
test('explicit save-and-reference recaptures the saved selection and never reuses its dirty token',async()=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.change();await h.controller.request({action:'captureSelection'})
  const token=h.lease().token,count=h.order.length
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:token,threadId,editable:false})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:token,threadId,editable:false,saveFirst:true})).toMatchObject({ok:true,view:{dirty:false,scope:{baseRevision:h.saved(),editable:false}}})
  const capture=h.calls.filter(c=>c.operation==='capture-selection')
  expect(capture).toHaveLength(1);expect(capture[0].input.selectionToken).not.toBe(token)
  expect(capture[0].input).toMatchObject({text:'selected',baseRevision:h.saved(),changeSequence:1})
  expect(h.order.slice(count).filter(item=>['engine:validateSelection','engine:freeze','engine:export','core:commit-object','engine:ack','engine:captureSelection','core:capture-selection'].includes(item))).toEqual(['engine:validateSelection','engine:freeze','engine:export','core:commit-object','engine:ack','engine:captureSelection','core:capture-selection'])
})
test.each(['freeze unavailable','empty recapture'] as const)('save-and-reference has no whole-document fallback after %s',async(kind)=>{
  const h=setupManual();await h.controller.request(open);await h.begin();h.change();await h.controller.request({action:'captureSelection'})
  const token=h.lease().token
  if(kind==='freeze unavailable')h.refuseFreeze();else h.emptyCapture()
  expect(await h.controller.request({action:'reference',...h.target(),selectionToken:token,threadId,editable:false,saveFirst:true})).toMatchObject({ok:false,error:kind==='freeze unavailable'?'native_input_unavailable':'unsupported_selection'})
  expect(h.calls.some(c=>c.operation==='capture-selection')).toBe(false)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(kind==='freeze unavailable'?0:1)
  expect(h.requests.some(r=>r.command==='replace')).toBe(false)
})
