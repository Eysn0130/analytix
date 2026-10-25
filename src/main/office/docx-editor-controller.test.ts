import { openDocumentRuntime } from '@superdoc/docx-engine/document-runtime'
import { z } from 'zod'
import { createHash } from 'node:crypto'
import { describe,it,expect } from 'vitest'
import { Document,Packer,Paragraph } from 'docx'
import { createDocxEditorController,verifyDocxCapture,replacedDocx,verifyDocxReplacement } from './docx-editor-controller'
import type {DocxEditorCapture} from '../../shared/docx-editor'
import type { PluginPackageView,PluginPackageHostRequest,PluginPackageHostResponse } from '../../../packages/runtime/src/contracts/plugin-package-host'
const hash=(bytes:Buffer)=>createHash('sha256').update(bytes).digest('hex')
const makeDoc=async(text:string)=>Packer.toBuffer(new Document({sections:[{children:[new Paragraph(text)]}]}))
async function fixture() {
  let disk=await makeDoc('Original'), session='a'.repeat(48), sequence=0
  let manual:Record<string,unknown>|undefined, approved:Record<string,unknown>|undefined, current:Record<string,unknown>|undefined, receipt:Record<string,unknown>|undefined
  let original=Buffer.from(disk),loseUndo=false,undoConflict=false,undoRefusal=false,foreignRecovery=false
  let interruptUndo=false,failRecoveries=0,failRecoveryAfterWrite=0,recoveryOverride:Record<string,unknown>={}
  let failUndoBeforeCore:'binding'|'transport'|undefined,failNextList=false
  let beginResponse:{reply:unknown;prepared:boolean}|undefined,canCancel=true
  let loseBegin=false,loseCommit=false,loseCancel=false,loseClose=false,loseOpen=false,conflict=false,externalOnClose:Buffer|undefined
  let sampled:Pick<DocxEditorCapture,'target'|'text'|'instanceId'|'captureId'>|undefined,scope:Record<string,unknown>|undefined
  const calls:Extract<PluginPackageHostRequest,{action:'invoke'}>[]=[]
  const pkg:PluginPackageView={packageId:'analytix-documents',packageVersion:'0.2.0',displayName:'Documents',origin:'development-source',publishable:false,
    materialized:true,generationId:'b'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'c'.repeat(64),available:true,
    operations:['open-object','close-object','object-status','commit-object','begin-manual-edit','object-recovery','undo-change','cancel-change','capture-selection','selection-read','proposal-read','proposal-accept','proposal-reject']}
  const host=async(request:PluginPackageHostRequest):Promise<PluginPackageHostResponse>=>{
    if(request.action==='list'){
      if(failNextList){failNextList=false;throw Error('binding failed before undo delivery')}
      return {ok:true,packages:[pkg]}
    }
    if(request.action!=='invoke')throw Error('unexpected package management')
    if(request.operation==='undo-change' && failUndoBeforeCore==='transport'){
      failUndoBeforeCore=undefined;throw Error('transport failed before undo delivery')
    }
    calls.push(request)
    const input=request.input as Record<string,unknown>
    let output:unknown
    if(request.operation!=='open-object' && (!session || input.sessionId!==session)) return {ok:true,output:{ok:false,code:'session_invalid',message:'Session absent'}} as PluginPackageHostResponse
    if(request.operation==='open-object'){
      if(!session)session=(++sequence).toString(16).padStart(48,'0')
      output={ok:true,document:{sessionId:session,objectId:'d'.repeat(64),path:'/workspace/report.docx',revision:hash(disk),content:disk.toString('base64')}}
      if(loseOpen){loseOpen=false;throw Error('lost open reply')}
    } else if(request.operation==='close-object'){
      session='';if(externalOnClose){disk=externalOnClose;externalOnClose=undefined}
      if(loseClose){loseClose=false;throw Error('lost close reply')}
      output={ok:true,closed:true}
    }
    else if(request.operation==='object-recovery'){
      if(failRecoveries>0){failRecoveries--;throw Error('lost recovery reply')}
      output={ok:true,recovery:{current:current?{...current,...recoveryOverride,...(foreignRecovery?{threadId:'other'}:{})}:null,pending:manual||approved?{...(manual?{...manual,version:2,origin:'manual'}:approved),threadId:'thread',revision:'',status:'prepared',canUndo:false,canCancel,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''}:null}}
      if(failUndoBeforeCore==='binding'){failUndoBeforeCore=undefined;failNextList=true}
    }
    else if(request.operation==='begin-manual-edit'){
      const response=beginResponse;beginResponse=undefined
      if((!response||response.prepared)&&!manual){original=Buffer.from(disk);manual={changeId:'e'.repeat(64),baseRevision:input.baseRevision,saveOperationId:'save_operation',undoOperationId:'undo_operation'}}
      if(response){if(response.reply instanceof Error)throw response.reply;return response.reply as PluginPackageHostResponse}
      if(loseBegin){loseBegin=false;throw Error('lost begin reply')}
      output={ok:true,manual:{...manual,version:2,origin:'manual'}}
    } else if(request.operation==='cancel-change'){manual=undefined;approved=undefined;if(loseCancel){loseCancel=false;throw Error('lost cancel reply')}output={ok:true,recovery:{current:current??null,pending:null}}}
    else if(request.operation==='commit-object'){
      const content=input.content as {data:string}
      if(!conflict){
        disk=Buffer.from(content.data,'base64')
        current={...(manual?{...manual,version:2,origin:'manual'}:approved),threadId:'thread',revision:hash(disk),status:'committed',canUndo:true,canCancel:false,canRetryUndo:false,canResume:false,createdAt:'',savedAt:'2026-10-17T00:00:00Z'}
        manual=undefined;approved=undefined
        failRecoveries=failRecoveryAfterWrite;failRecoveryAfterWrite=0
      }
      receipt={operationId:input.operationId,revision:conflict?'':hash(disk),status:conflict?'conflict':'committed',savedAt:conflict?'':'2026-10-17T00:00:00Z'}
      if(loseCommit){loseCommit=false;throw Error('lost commit reply')}
      output=conflict?{ok:false,code:'conflict',message:'Conflict',receipt}:{ok:true,receipt}
    } else if(request.operation==='undo-change'){
      expect(input).toEqual({sessionId:session,threadId:'thread',changeId:current!.changeId,baseRevision:current!.revision})
      if(undoRefusal){undoRefusal=false;output={ok:false,code:'forbidden',message:'Synthetic refusal'}}
      else if(interruptUndo){
        interruptUndo=false;current={...current,status:'unknown',canUndo:false,canRetryUndo:true}
        receipt={operationId:current.undoOperationId,revision:'',status:'pending',savedAt:''}
        throw Error('undo interrupted before its journal')
      }
      else {
        const collided=undoConflict||hash(disk)!==current!.revision
        receipt={operationId:current!.undoOperationId,revision:collided?'':hash(original),status:collided?'conflict':'committed',savedAt:'2026-10-17T00:01:00Z'}
        if(!collided){disk=Buffer.from(original);current={...current,revision:hash(original),status:'undone',canUndo:false,canRetryUndo:false};failRecoveries=failRecoveryAfterWrite;failRecoveryAfterWrite=0}
        if(loseUndo){loseUndo=false;throw Error('lost undo reply')}
        output=collided?{ok:false,code:'conflict',message:'Conflict',receipt}:{ok:true,receipt}
      }
    } else if(request.operation==='object-status')output=receipt?.operationId===input.operationId?{ok:true,receipt}:{ok:false,code:'operation_not_found',message:'No journal for this operation'}
    else if(request.operation==='capture-selection'){
      const {text,...authority}=input;scope={...authority,scopeId:'f'.repeat(48),parts:[{kind:'literal',text}]};output={ok:true,scope}
    } else if(request.operation==='selection-read')output={ok:true,scope}
    else if(request.operation==='proposal-read')output={ok:true,proposals:[],localReviews:[]}
    else if(request.operation==='proposal-accept'){
      original=Buffer.from(disk)
      approved={changeId:'e'.repeat(64),proposalId:input.proposalId,beforeText:'Original',afterText:'Approved',baseRevision:hash(disk),saveOperationId:'approved_save_operation',undoOperationId:'approved_undo_operation'}
      output={ok:true,replacement:{proposalId:input.proposalId,operationId:input.operationId,
        text:'Approved',changeId:'e'.repeat(64),saveOperationId:'approved_save_operation',selectionToken:scope!.selectionToken,changeSequence:0,baseRevision:hash(disk)}}
    }
    else throw Error('unexpected '+request.operation)
    return {ok:true,output} as PluginPackageHostResponse
  }
  const controller=createDocxEditorController(host,async view=>sampled?{...sampled,requestId:'00000000-0000-4000-8000-000000000010',editorId:view.editorId,version:view.version,revision:view.revision}:null)
  const opened=await controller.request({action:'open',workspace:'/workspace',path:'report.docx',threadId:'thread'})
  expect(opened.ok).toBe(true)
  const editorId=opened.view!.editorId
  const bytes=await makeDoc('Edited')
  const content={encoding:'base64',kind:'docx',byteLength:bytes.length,sha256:hash(bytes),data:bytes.toString('base64')}
  return {controller,editorId,content,calls,pkg,failUndoDelivery:(stage:'binding'|'transport')=>{failUndoBeforeCore=stage},interruptUndo:()=>{interruptUndo=true},overrideRecovery:(value:Record<string,unknown>)=>{recoveryOverride=value},failFinalRecovery:(count=1)=>{failRecoveryAfterWrite=count},loseUndo:()=>{loseUndo=true},undoConflict:()=>{undoConflict=true},undoRefusal:()=>{undoRefusal=true},foreignRecovery:()=>{foreignRecovery=true},externalEdit:(bytes:Buffer)=>{disk=bytes},beginResponse:(reply:unknown,prepared=false)=>{beginResponse={reply,prepared}},seedPending:()=>{manual={changeId:'e'.repeat(64),baseRevision:hash(disk),saveOperationId:'save_operation',undoOperationId:'undo_operation'}},pending:()=>!!manual,refuseCancel:()=>{canCancel=false},sample:(value:NonNullable<typeof sampled>)=>{sampled=value},disk:()=>disk,loseBegin:()=>{loseBegin=true},loseCommit:()=>{loseCommit=true},loseCancel:()=>{loseCancel=true},loseClose:()=>{loseClose=true},loseOpen:()=>{loseOpen=true},externalOnClose:(bytes:Buffer)=>{externalOnClose=bytes},session:()=>session,conflict:()=>{conflict=true}}
}
describe('ordinary DOCX byte bridge',()=>{
  it('saves real DOCX bytes through the reserved Core operation and reopens the receipt revision',async()=>{
    const h=await fixture()
    expect((await h.controller.request({action:'begin',editorId:h.editorId,version:0})).ok).toBe(true)
    await h.controller.request({action:'changed',editorId:h.editorId,version:1})
    expect(await h.controller.request({action:'close',editorId:h.editorId})).toMatchObject({ok:false,error:'unsaved_changes'})
    const result=await h.controller.request({action:'save',editorId:h.editorId,version:1,content:h.content})
    expect(result).toMatchObject({ok:true,view:{phase:'ready',dirty:false,revision:h.content.sha256,canUndo:true,recovery:{current:{version:2,origin:'manual',changeId:'e'.repeat(64)}}}})
    expect(h.disk().toString('base64')).toBe(h.content.data)
    const commit=h.calls.find(c=>c.operation==='commit-object')!
    expect(commit.input).toMatchObject({operationId:'save_operation',changeId:'e'.repeat(64),content:h.content})
    expect(h.calls.at(-2)?.operation).toBe('open-object')
    expect(h.calls.at(-1)?.operation).toBe('object-recovery')
    expect((await h.controller.request({action:'close',editorId:h.editorId})).ok).toBe(true)
  })
  it('replays one lost begin identity and only queries a lost commit, never submits again',async()=>{
    const h=await fixture();h.loseBegin()
    expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown'})
    expect((await h.controller.request({action:'status',editorId:h.editorId})).ok).toBe(true)
    const begins=h.calls.filter(c=>c.operation==='begin-manual-edit')
    expect(begins).toHaveLength(2);expect(begins[0].input).toEqual(begins[1].input)
    h.loseCommit()
    expect(await h.controller.request({action:'save',editorId:h.editorId,version:0,content:h.content})).toMatchObject({ok:false,error:'unknown'})
    expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toMatchObject({ok:false,error:'unknown'})
    expect((await h.controller.request({action:'status',editorId:h.editorId})).ok).toBe(true)
    expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
    expect(h.calls.filter(c=>c.operation==='object-status')).toHaveLength(1)
  })
  it('keeps a conflict and rejects stale edits and arbitrary path saves without overwriting',async()=>{
    const h=await fixture(),before=h.disk();await h.controller.request({action:'begin',editorId:h.editorId,version:0})
    await h.controller.request({action:'changed',editorId:h.editorId,version:2})
    expect(await h.controller.request({action:'save',editorId:h.editorId,version:1,content:h.content})).toMatchObject({ok:false,error:'stale_selection'})
    expect(await h.controller.request({action:'save',editorId:h.editorId,version:2,path:'/elsewhere',content:h.content})).toMatchObject({ok:false,error:'invalid_request'})
    h.conflict()
    expect(await h.controller.request({action:'save',editorId:h.editorId,version:2,content:h.content})).toMatchObject({ok:false,error:'conflict',view:{dirty:true}})
    expect(h.disk()).toBe(before)
    expect((await h.controller.request({action:'close',editorId:h.editorId,discard:true})).ok).toBe(true)
    expect(h.calls.some(c=>c.operation==='cancel-change')).toBe(true)
  })
  it('rejects caller-supplied selection text and cannot mint a scope without a bound capture producer',async()=>{
    const h=await fixture()
    const capture={action:'capture',editorId:h.editorId,version:0,instanceId:'00000000-0000-4000-8000-000000000002',captureId:'00000000-0000-4000-8000-000000000003',editable:true}
    expect(await h.controller.request({...capture,text:'forged'})).toMatchObject({ok:false,error:'invalid_request'})
    expect(await h.controller.request(capture)).toMatchObject({ok:false,error:'stale_selection'})
    expect(h.calls.some(c=>c.operation==='capture-selection')).toBe(false)
  })
  it('does not bypass disabled package readiness or freeze',async()=>{
    const h=await fixture();h.pkg.desiredState='disabled'
    expect((await h.controller.request({action:'begin',editorId:h.editorId,version:0})).ok).toBe(false)
    h.controller.freeze(true)
    expect((await h.controller.request({action:'open',workspace:'/workspace',path:'other.docx',threadId:'thread'})).ok).toBe(false)
    expect(h.calls.some(c=>c.operation==='begin-manual-edit')).toBe(false)
  })
})

it('uses the published engine against Core bytes to reject forged range text and foreign targets',async()=>{
  const bytes=await makeDoc('Original')
  const runtime=await openDocumentRuntime({bytes})
  let target:Record<string,unknown>
  try {
    const raw=await runtime.invoke({operationId:'query.match',input:{select:{type:'text',pattern:'Original'}}})
    const found=z.object({items:z.array(z.object({target:z.record(z.string(),z.unknown())}))}).parse(raw)
    target=found.items[0].target
  } finally {await runtime.close()}
  const view={editorId:'00000000-0000-4000-8000-000000000001',workspace:'/workspace',path:'/workspace/report.docx',threadId:'thread',objectId:'a'.repeat(64),revision:hash(bytes),
    version:0,phase:'ready' as const,dirty:false,content:{encoding:'base64' as const,kind:'docx' as const,byteLength:bytes.length,sha256:hash(bytes),data:bytes.toString('base64')},proposals:[],reviews:[]}
  const capture={requestId:'00000000-0000-4000-8000-000000000002',editorId:view.editorId,version:0,revision:view.revision,
    instanceId:'00000000-0000-4000-8000-000000000003',captureId:'00000000-0000-4000-8000-000000000004',text:'Original',target}
  expect(await verifyDocxCapture(view,capture)).toBe(true)
  expect(await verifyDocxCapture(view,{...capture,text:'Forged'})).toBe(false)
  const expected=await replacedDocx(view,target,'Approved','Original')
  expect(await verifyDocxReplacement(expected,await replacedDocx(view,target,'Approved','Original'))).toBe(true)
  expect(await verifyDocxReplacement(expected,await makeDoc('Unapproved document'))).toBe(false)
  await expect(verifyDocxCapture(view,{...capture,target:{...target,start:{kind:'text',blockId:'foreign',offset:0}}})).rejects.toBeDefined()
})

it('retries the same lost cancellation and refuses a workspace-alias second owner',async()=>{
  const h=await fixture()
  expect(await h.controller.request({action:'open',workspace:'/workspace/.',path:'report.docx',threadId:'other'})).toMatchObject({ok:false,error:'unsaved_changes'})
  await h.controller.request({action:'begin',editorId:h.editorId,version:0});h.loseCancel()
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toMatchObject({ok:false,view:{phase:'unknown'}})
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown'})
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toEqual({ok:true,view:null})
  const calls=h.calls.filter(c=>c.operation==='cancel-change')
  expect(calls).toHaveLength(2);expect(calls[0].input).toEqual(calls[1].input)
})

it('binds an approved range, replays its ACK and refuses unrelated whole-document edits before Core commit',async()=>{
  const h=await fixture(),opened=await h.controller.request({action:'open',workspace:'/workspace',path:'report.docx',threadId:'thread'})
  const view=opened.view!,runtime=await openDocumentRuntime({bytes:h.disk()})
  let target:Record<string,unknown>
  try {const raw=await runtime.invoke({operationId:'query.match',input:{select:{type:'text',pattern:'Original'}}})
    target=z.object({items:z.array(z.object({target:z.record(z.string(),z.unknown())}))}).parse(raw).items[0].target
  }finally{await runtime.close()}
  const capture={instanceId:'00000000-0000-4000-8000-000000000003',captureId:'00000000-0000-4000-8000-000000000004'}
  h.sample({...capture,target,text:'Original'})
  expect(await h.controller.request({action:'capture',editorId:h.editorId,version:0,...capture,editable:true})).toMatchObject({ok:true,view:{scope:{editable:true}}})
  const accepted=await h.controller.request({action:'decide',editorId:h.editorId,version:0,...capture,scopeId:'f'.repeat(48),proposalId:'9'.repeat(48),accept:true})
  expect(accepted).toMatchObject({ok:true,view:{phase:'applying',dirty:true},replacement:'Approved'})
  expect(await h.controller.request({action:'save',editorId:h.editorId,version:0,content:h.content})).toMatchObject({ok:false,error:'unsaved_changes'})
  const ack={action:'applied',editorId:h.editorId,version:0,...capture}
  expect(await h.controller.request(ack)).toMatchObject({ok:true,view:{phase:'editing',version:1}})
  expect(await h.controller.request(ack)).toMatchObject({ok:true,view:{phase:'editing',version:1}})
  expect(await h.controller.request({...ack,captureId:'00000000-0000-4000-8000-000000000005'})).toMatchObject({ok:false,error:'stale_selection'})
  expect(await h.controller.request({action:'save',editorId:h.editorId,version:1,content:h.content})).toMatchObject({ok:false,error:'forbidden'})
  expect(h.calls.some(c=>c.operation==='commit-object')).toBe(false)
  const bytes=Buffer.from(await replacedDocx(view,target,'Approved','Original'))
  const content={encoding:'base64',kind:'docx',byteLength:bytes.length,sha256:hash(bytes),data:bytes.toString('base64')}
  expect(await h.controller.request({action:'save',editorId:h.editorId,version:1,content})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:true}})
  expect(h.calls.find(c=>c.operation==='commit-object')?.input).toMatchObject({operationId:'approved_save_operation',changeId:'e'.repeat(64),content})
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:1})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:false,version:2,revision:view.revision}})
  expect(h.calls.find(c=>c.operation==='undo-change')?.input).toMatchObject({changeId:'e'.repeat(64),baseRevision:content.sha256})
  expect(h.disk().toString('base64')).toBe(view.content.data)
})

it('recovers a lost close ACK by retrying its exact old handle before reopening',async()=>{
  const h=await fixture(),old=h.session();h.loseClose()
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown'}})
  expect(h.controller.hasUnsettled()).toBe(true)
  expect(h.calls.some(c=>['begin-manual-edit','commit-object'].includes(c.operation))).toBe(false)
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:true,view:{phase:'ready'}})
  const closes=h.calls.filter(c=>c.operation==='close-object')
  expect(closes).toHaveLength(2);expect(closes.map(c=>c.input)).toEqual([{sessionId:old},{sessionId:old}])
  expect(h.session()).not.toBe(old)
  expect((await h.controller.request({action:'begin',editorId:h.editorId,version:0})).ok).toBe(true)
})
it('retries only open after a new Core handle was allocated but its reply was lost',async()=>{
  const h=await fixture();h.loseOpen()
  expect(await h.controller.request({action:'revoke',editorId:h.editorId})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown'}})
  const allocated=h.session(),before=h.calls.length
  expect(allocated).not.toBe('')
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:true,view:{phase:'ready'}})
  expect(h.session()).toBe(allocated)
  expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['open-object','object-recovery'])
  expect(h.calls.some(c=>['begin-manual-edit','commit-object'].includes(c.operation))).toBe(false)
})
it.each(['begin','save'] as const)('keeps the editor baseline on %s reopen conflict and safely discards through the acquired new handle',async action=>{
  const h=await fixture(),external=await makeDoc('External document')
  if(action==='save')await h.controller.request({action:'begin',editorId:h.editorId,version:0})
  const retained=action==='save'?h.content.data:h.disk().toString('base64')
  h.externalOnClose(external)
  const result=await h.controller.request(action==='begin'?{action,editorId:h.editorId,version:0}:{action,editorId:h.editorId,version:0,content:h.content})
  expect(result).toMatchObject({ok:false,error:'conflict',view:{phase:'conflict',content:{data:retained}}})
  const acquired=h.session(),before=h.calls.length
  expect(acquired).not.toBe('')
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toEqual({ok:true,view:null})
  expect(h.calls.slice(before).map(c=>[c.operation,(c.input as {sessionId:string}).sessionId])).toEqual([['object-recovery',acquired],['close-object',acquired]])
  expect(h.disk()).toBe(external)
  const reopened=await h.controller.request({action:'open',workspace:'/workspace',path:'report.docx',threadId:'thread'})
  expect(reopened).toMatchObject({ok:true,view:{phase:'ready',revision:hash(external),content:{data:external.toString('base64')}}})
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(action==='save'?1:0)
})
it.each(['status','close'] as const)('finishes a lost terminal close via %s without recovery on the retired handle',async action=>{
  const h=await fixture();h.loseClose()
  expect(await h.controller.request({action:'close',editorId:h.editorId})).toMatchObject({ok:false,error:'unknown'})
  const before=h.calls.length
  expect(await h.controller.request({action,editorId:h.editorId, ...(action==='close'?{discard:true}:{})})).toEqual({ok:true,view:null})
  expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['close-object'])
  expect(h.controller.views()).toEqual([])
  expect(h.calls.every(c=>c.operation==='open-object' || (c.input as {sessionId:string}).sessionId!=='')).toBe(true)
})

it.each(['conflict','forbidden'] as const)('retires only a validated manual begin refusal %s and permits close/reopen',async code=>{
  const h=await fixture(),before=h.disk()
  h.beginResponse({ok:true,output:{ok:false,code,message:'Synthetic refusal'}})
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:code,view:{phase:code==='conflict'?'conflict':'ready',dirty:false}})
  expect(h.controller.hasUnsettled()).toBe(false)
  const rejected=h.calls.find(c=>c.operation==='begin-manual-edit')!.input
  await h.controller.request({action:'status',editorId:h.editorId})
  expect(h.calls.filter(c=>c.operation==='begin-manual-edit')).toHaveLength(1)
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toEqual({ok:true,view:null})
  expect(h.calls.some(c=>['commit-object','cancel-change'].includes(c.operation))).toBe(false)
  expect(h.disk()).toBe(before)
  const reopened=await h.controller.request({action:'open',workspace:'/workspace',path:'report.docx',threadId:'thread'})
  expect(reopened.ok).toBe(true)
  expect(await h.controller.request({action:'begin',editorId:reopened.view!.editorId,version:0})).toMatchObject({ok:true,view:{phase:'editing'}})
  const begins=h.calls.filter(c=>c.operation==='begin-manual-edit')
  expect(begins).toHaveLength(2)
  expect((begins[1].input as {operationId:string}).operationId).not.toBe((rejected as {operationId:string}).operationId)
  expect(h.disk()).toBe(before)
})
it.each([
  {name:'timeout',reply:new Error('Synthetic timeout')},
  {name:'invalid outer response',reply:{ok:false,code:'unvalidated_code',message:'Synthetic'}},
  {name:'invalid inner response',reply:{ok:true,output:{ok:false,code:'unvalidated_code',message:'Synthetic'}}},
  {name:'outer conflict',reply:{ok:false,code:'conflict',message:'Synthetic'}},
  {name:'persistence failure',reply:{ok:true,output:{ok:false,code:'persistence_failure',message:'Synthetic'}}},
  {name:'conflict with uncertain receipt',reply:{ok:true,output:{ok:false,code:'conflict',message:'Synthetic',receipt:{operationId:'save_operation',revision:'',status:'unknown',savedAt:''}}}},
  {name:'invalid manual contract',reply:{ok:true,output:{ok:true,manual:{version:2,origin:'manual'}}}},
])('retains uncertain manual begin $name with its exact operation identity',async({reply})=>{
  const h=await fixture(),before=h.disk()
  h.beginResponse(reply,true)
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown'}})
  expect(h.controller.hasUnsettled()).toBe(true);expect(h.pending()).toBe(true)
  const first=h.calls.find(c=>c.operation==='begin-manual-edit')!.input,count=h.calls.length
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toMatchObject({ok:false,error:'unknown'})
  expect(h.calls).toHaveLength(count);expect(h.pending()).toBe(true)
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:true,view:{phase:'editing'}})
  const begins=h.calls.filter(c=>c.operation==='begin-manual-edit')
  expect(begins).toHaveLength(2);expect(begins[1].input).toEqual(first)
  expect(h.calls.some(c=>['commit-object','cancel-change'].includes(c.operation))).toBe(false)
  expect(h.disk()).toBe(before)
})
it('does not cancel a durable pending change after a validated begin refusal without Core permission',async()=>{
  const h=await fixture(),before=h.disk()
  h.seedPending();h.refuseCancel();h.beginResponse({ok:true,output:{ok:false,code:'conflict',message:'Synthetic refusal'}})
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'conflict'})
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toMatchObject({ok:false,error:'unknown'})
  expect(h.pending()).toBe(true)
  expect(h.calls.filter(c=>c.operation==='object-recovery')).toHaveLength(2)
  expect(h.calls.some(c=>['commit-object','cancel-change'].includes(c.operation))).toBe(false)
  expect(h.disk()).toBe(before)
})

it('does not cancel a durable pending on normal close or destroy after a refusal before explicit discard',async()=>{
  const h=await fixture(),before=h.disk()
  h.seedPending();h.beginResponse({ok:true,output:{ok:false,code:'conflict',message:'Synthetic refusal'}})
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'conflict'})
  expect(await h.controller.request({action:'close',editorId:h.editorId})).toMatchObject({ok:false,error:'unknown'})
  await expect(h.controller.destroy()).rejects.toThrow('unknown')
  expect(h.pending()).toBe(true)
  expect(h.calls.some(c=>['commit-object','cancel-change'].includes(c.operation))).toBe(false)
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toEqual({ok:true,view:null})
  expect(h.pending()).toBe(false)
  expect(h.calls.filter(c=>c.operation==='cancel-change')).toHaveLength(1)
  expect(h.disk()).toBe(before)
})

async function savedManual() {
  const h=await fixture(),original=Buffer.from(h.disk())
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:true,view:{canUndo:false}})
  expect(await h.controller.request({action:'save',editorId:h.editorId,version:0,content:h.content})).toMatchObject({ok:true,view:{canUndo:true}})
  return {...h,original}
}

it('undoes a saved manual change using only Core change authority and makes stale repeated requests harmless',async()=>{
  const h=await savedManual()
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0,path:'/other',content:h.content})).toMatchObject({ok:false,error:'invalid_request'})
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:false,version:1,revision:hash(h.original),recovery:{current:{status:'undone'}}}})
  expect(h.disk()).toEqual(h.original)
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'stale_selection'})
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:1})).toMatchObject({ok:false,error:'invalid_request',view:{phase:'ready'}})
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
})

it('queries a lost saved undo receipt by the original operation without replaying undo or exporting another save',async()=>{
  const h=await savedManual();h.loseUndo()
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',dirty:false,canUndo:false,version:0}})
  const before=h.calls.length
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toMatchObject({ok:false,error:'unknown'})
  expect(h.calls).toHaveLength(before)
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:false,version:1,revision:hash(h.original)}})
  expect(h.calls.filter(c=>c.operation==='object-status').map(c=>c.input)).toEqual([{sessionId:expect.any(String),operationId:'undo_operation'}])
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
  expect(h.disk()).toEqual(h.original)
})

it('retains the saved editor on undo CAS conflict and never reloads a rejected rollback',async()=>{
  const h=await savedManual(),saved=Buffer.from(h.disk()),before=h.calls.length;h.undoConflict()
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'conflict',view:{phase:'conflict',dirty:false,canUndo:false,revision:h.content.sha256,content:{data:h.content.data}}})
  expect(h.disk()).toEqual(saved)
  expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['object-recovery','undo-change'])
  expect(await h.controller.request({action:'close',editorId:h.editorId})).toEqual({ok:true,view:null})
})

it('preserves an external source update when a saved undo reaches the Core CAS check',async()=>{
  const h=await savedManual(),external=await makeDoc('External source edit')
  h.externalEdit(external)
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'conflict',view:{phase:'conflict',canUndo:false,revision:h.content.sha256,content:{data:h.content.data}}})
  expect(h.disk()).toEqual(external)
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
})

it('does not treat a historical undo receipt as proof after another application edits the file',async()=>{
  const h=await savedManual();h.loseUndo()
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown'})
  const external=await makeDoc('External document');h.externalEdit(external)
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:false,error:'conflict',view:{phase:'conflict',dirty:false,canUndo:false,revision:h.content.sha256,content:{data:h.content.data}}})
  expect(h.disk()).toEqual(external)
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
})

it('finishes an interrupted reload of a committed undo without another rollback or status write',async()=>{
  const h=await savedManual();h.loseClose()
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',dirty:false,version:1}})
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:false,version:1,revision:hash(h.original)}})
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
  expect(h.calls.filter(c=>c.operation==='object-status')).toHaveLength(0)
})

it('releases a proven undo refusal and rejects dirty, frozen or foreign-thread recovery before a write',async()=>{
  const h=await savedManual();h.undoRefusal()
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'forbidden',view:{phase:'ready',canUndo:true}})
  h.controller.freeze(true)
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unavailable'})
  h.controller.freeze(false);h.foreignRecovery()
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unavailable',view:{canUndo:false,recovery:{current:{threadId:'thread'}}}})
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
  const dirty=await savedManual();await dirty.controller.request({action:'begin',editorId:dirty.editorId,version:0})
  await dirty.controller.request({action:'changed',editorId:dirty.editorId,version:1})
  expect(await dirty.controller.request({action:'undoChange',editorId:dirty.editorId,version:1})).toMatchObject({ok:false,error:'unsaved_changes',view:{dirty:true}})
  expect(dirty.calls.filter(c=>c.operation==='undo-change')).toHaveLength(0)
})

it('rediscovers saved undo after close and reopen without any renderer-retained change identity',async()=>{
  const h=await savedManual()
  expect(await h.controller.request({action:'close',editorId:h.editorId})).toEqual({ok:true,view:null})
  const opened=await h.controller.request({action:'open',workspace:'/workspace',path:'report.docx',threadId:'thread'})
  expect(opened).toMatchObject({ok:true,view:{canUndo:true,recovery:{current:{changeId:'e'.repeat(64)}}}})
  expect(await h.controller.request({action:'undoChange',editorId:opened.view!.editorId,version:0})).toMatchObject({ok:true,view:{canUndo:false,revision:hash(h.original)}})
  expect(h.disk()).toEqual(h.original)
})

it.each(['save','undoChange'] as const)('keeps a failed final %s recovery refresh queryable until metadata is confirmed',async action=>{
  const h=action==='save'?await fixture():await savedManual()
  if(action==='save')await h.controller.request({action:'begin',editorId:h.editorId,version:0})
  h.failFinalRecovery(2)
  const expected=action==='save'?h.content.sha256:hash(h.disk())
  const result=await h.controller.request(action==='save'?{action,editorId:h.editorId,version:0,content:h.content}:{action,editorId:h.editorId,version:0})
  expect(result).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',dirty:false,canUndo:false}})
  if(action==='save')expect(result.view!.revision).toBe(expected)
  else expect(result.view!.revision).not.toBe(expected)
  expect(h.controller.hasUnsettled()).toBe(true)
  const before=h.calls.length
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:result.view!.version})).toMatchObject({ok:false,error:'unknown'})
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',dirty:false}})
  expect(h.controller.hasUnsettled()).toBe(true)
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:action==='save'}})
  expect(h.controller.hasUnsettled()).toBe(false)
  expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['object-recovery','object-recovery'])
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(action==='undoChange'?1:0)
})

it.each([
  {stage:'binding',next:'retry'},
  {stage:'transport',next:'retry'},
  {stage:'binding',next:'close'},
  {stage:'transport',next:'close'},
] as const)('recovers an undo $stage failure before Core delivery to ready for explicit $next',async({stage,next})=>{
  const h=await savedManual();h.failUndoDelivery(stage)
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',dirty:false,canUndo:false,version:0}})
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(0)
  expect(h.controller.hasUnsettled()).toBe(true)
  const before=h.calls.length
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:true,version:0,revision:h.content.sha256,content:{data:h.content.data},recovery:{current:{status:'committed',canUndo:true}}}})
  expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['object-status','object-recovery'])
  expect(h.calls[before].input).toMatchObject({operationId:'undo_operation'})
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(0)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
  expect(h.disk().toString('base64')).toBe(h.content.data)
  expect(h.controller.hasUnsettled()).toBe(false)
  if(next==='retry'){
    expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:false,version:1,revision:hash(h.original)}})
    expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
    expect(h.disk()).toEqual(h.original)
  }else{
    expect(await h.controller.request({action:'close',editorId:h.editorId})).toEqual({ok:true,view:null})
    expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['object-status','object-recovery','object-recovery','close-object'])
    expect(h.disk().toString('base64')).toBe(h.content.data)
  }
})

it.each([
  {changeId:'9'.repeat(64)},
  {undoOperationId:'other_undo_operation'},
  {baseRevision:'9'.repeat(64)},
  {revision:'9'.repeat(64)},
  {canUndo:false},
])('retains an undelivered undo when the committed recovery does not match %j',async mismatch=>{
  const h=await savedManual();h.failUndoDelivery('transport')
  await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})
  h.overrideRecovery(mismatch)
  const before=h.calls.length
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:false,view:{phase:'unknown',canUndo:false}})
  expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['object-status','object-recovery'])
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toMatchObject({ok:false,error:'unknown'})
  expect(h.controller.hasUnsettled()).toBe(true)
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(0)
  expect(h.disk().toString('base64')).toBe(h.content.data)
})

it('retains an undelivered undo while Core reports another pending change',async()=>{
  const h=await savedManual();h.failUndoDelivery('binding')
  await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})
  h.seedPending()
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:false,view:{phase:'unknown',canUndo:false}})
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toMatchObject({ok:false,error:'unknown'})
  expect(h.controller.hasUnsettled()).toBe(true)
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(0)
  expect(h.disk().toString('base64')).toBe(h.content.data)
})

it('queries an interrupted undo before exposing only a later explicit completion with the same Core identity',async()=>{
  const h=await savedManual();h.interruptUndo()
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',canUndo:false}})
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unsaved_changes'})
  const before=h.calls.length
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',canUndo:true,recovery:{current:{status:'unknown',canRetryUndo:true}}}})
  expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['object-status','object-recovery'])
  expect(h.calls[before].input).toMatchObject({operationId:'undo_operation'})
  expect(h.disk().toString('base64')).toBe(h.content.data)
  expect(h.controller.hasUnsettled()).toBe(true)
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',canUndo:true}})
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
  expect(await h.controller.request({action:'begin',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unknown'})
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:true,view:{phase:'ready',dirty:false,canUndo:false,version:1,revision:hash(h.original)}})
  const undos=h.calls.filter(c=>c.operation==='undo-change')
  expect(undos).toHaveLength(2);expect(undos[1].input).toEqual(undos[0].input)
  expect(h.disk()).toEqual(h.original)
  expect(h.calls.filter(c=>c.operation==='commit-object')).toHaveLength(1)
})

it.each([
  {changeId:'9'.repeat(64)},
  {undoOperationId:'other_undo_operation'},
  {baseRevision:'9'.repeat(64)},
  {revision:'9'.repeat(64)},
])('retains the original pending undo when Core retry recovery does not match %j',async mismatch=>{
  const h=await savedManual();h.interruptUndo()
  await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})
  h.overrideRecovery(mismatch)
  expect(await h.controller.request({action:'status',editorId:h.editorId})).toMatchObject({ok:false,error:'unknown',view:{phase:'unknown',canUndo:false}})
  expect(await h.controller.request({action:'undoChange',editorId:h.editorId,version:0})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(await h.controller.request({action:'close',editorId:h.editorId,discard:true})).toMatchObject({ok:false,error:'unknown'})
  expect(h.calls.filter(c=>c.operation==='undo-change')).toHaveLength(1)
  expect(h.disk().toString('base64')).toBe(h.content.data)
})
