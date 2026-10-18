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
  let manual:Record<string,unknown>|undefined, receipt:Record<string,unknown>|undefined
  let beginResponse:{reply:unknown;prepared:boolean}|undefined,canCancel=true
  let loseBegin=false,loseCommit=false,loseCancel=false,loseClose=false,loseOpen=false,conflict=false,externalOnClose:Buffer|undefined
  let sampled:Pick<DocxEditorCapture,'target'|'text'|'instanceId'|'captureId'>|undefined,scope:Record<string,unknown>|undefined
  const calls:Extract<PluginPackageHostRequest,{action:'invoke'}>[]=[]
  const pkg:PluginPackageView={packageId:'analytix-documents',packageVersion:'0.2.0',displayName:'Documents',origin:'development-source',publishable:false,
    materialized:true,generationId:'b'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'c'.repeat(64),available:true,
    operations:['open-object','close-object','object-status','commit-object','begin-manual-edit','object-recovery','cancel-change','capture-selection','selection-read','proposal-read','proposal-accept','proposal-reject']}
  const host=async(request:PluginPackageHostRequest):Promise<PluginPackageHostResponse>=>{
    if(request.action==='list')return {ok:true,packages:[pkg]}
    if(request.action!=='invoke')throw Error('unexpected package management')
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
    else if(request.operation==='object-recovery')output={ok:true,recovery:{current:null,pending:manual?{...manual,version:2,origin:'manual',threadId:'thread',revision:'',status:'prepared',canUndo:false,canCancel,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''}:null}}
    else if(request.operation==='begin-manual-edit'){
      const response=beginResponse;beginResponse=undefined
      if(!response||response.prepared)manual??={changeId:'e'.repeat(64),baseRevision:input.baseRevision,saveOperationId:'save_operation',undoOperationId:'undo_operation'}
      if(response){if(response.reply instanceof Error)throw response.reply;return response.reply as PluginPackageHostResponse}
      if(loseBegin){loseBegin=false;throw Error('lost begin reply')}
      output={ok:true,manual:{...manual,version:2,origin:'manual'}}
    } else if(request.operation==='cancel-change'){manual=undefined;if(loseCancel){loseCancel=false;throw Error('lost cancel reply')}output={ok:true,recovery:{current:null,pending:null}}}
    else if(request.operation==='commit-object'){
      const content=input.content as {data:string}
      if(!conflict){disk=Buffer.from(content.data,'base64');manual=undefined}
      receipt={operationId:input.operationId,revision:conflict?'':hash(disk),status:conflict?'conflict':'committed',savedAt:conflict?'':'2026-10-17T00:00:00Z'}
      if(loseCommit){loseCommit=false;throw Error('lost commit reply')}
      output=conflict?{ok:false,code:'conflict',message:'Conflict',receipt}:{ok:true,receipt}
    } else if(request.operation==='object-status')output={ok:true,receipt}
    else if(request.operation==='capture-selection'){
      const {text,...authority}=input;scope={...authority,scopeId:'f'.repeat(48),parts:[{kind:'literal',text}]};output={ok:true,scope}
    } else if(request.operation==='selection-read')output={ok:true,scope}
    else if(request.operation==='proposal-read')output={ok:true,proposals:[],localReviews:[]}
    else if(request.operation==='proposal-accept')output={ok:true,replacement:{proposalId:input.proposalId,operationId:input.operationId,
      text:'Approved',changeId:'e'.repeat(64),saveOperationId:'approved_save_operation',selectionToken:scope!.selectionToken,changeSequence:0,baseRevision:hash(disk)}}
    else throw Error('unexpected '+request.operation)
    return {ok:true,output} as PluginPackageHostResponse
  }
  const controller=createDocxEditorController(host,async view=>sampled?{...sampled,requestId:'00000000-0000-4000-8000-000000000010',editorId:view.editorId,version:view.version,revision:view.revision}:null)
  const opened=await controller.request({action:'open',workspace:'/workspace',path:'report.docx',threadId:'thread'})
  expect(opened.ok).toBe(true)
  const editorId=opened.view!.editorId
  const bytes=await makeDoc('Edited')
  const content={encoding:'base64',kind:'docx',byteLength:bytes.length,sha256:hash(bytes),data:bytes.toString('base64')}
  return {controller,editorId,content,calls,pkg,beginResponse:(reply:unknown,prepared=false)=>{beginResponse={reply,prepared}},seedPending:()=>{manual={changeId:'e'.repeat(64),baseRevision:hash(disk),saveOperationId:'save_operation',undoOperationId:'undo_operation'}},pending:()=>!!manual,refuseCancel:()=>{canCancel=false},sample:(value:NonNullable<typeof sampled>)=>{sampled=value},disk:()=>disk,loseBegin:()=>{loseBegin=true},loseCommit:()=>{loseCommit=true},loseCancel:()=>{loseCancel=true},loseClose:()=>{loseClose=true},loseOpen:()=>{loseOpen=true},externalOnClose:(bytes:Buffer)=>{externalOnClose=bytes},session:()=>session,conflict:()=>{conflict=true}}
}
describe('ordinary DOCX byte bridge',()=>{
  it('saves real DOCX bytes through the reserved Core operation and reopens the receipt revision',async()=>{
    const h=await fixture()
    expect((await h.controller.request({action:'begin',editorId:h.editorId,version:0})).ok).toBe(true)
    await h.controller.request({action:'changed',editorId:h.editorId,version:1})
    expect(await h.controller.request({action:'close',editorId:h.editorId})).toMatchObject({ok:false,error:'unsaved_changes'})
    const result=await h.controller.request({action:'save',editorId:h.editorId,version:1,content:h.content})
    expect(result).toMatchObject({ok:true,view:{phase:'ready',dirty:false,revision:h.content.sha256}})
    expect(h.disk().toString('base64')).toBe(h.content.data)
    const commit=h.calls.find(c=>c.operation==='commit-object')!
    expect(commit.input).toMatchObject({operationId:'save_operation',changeId:'e'.repeat(64),content:h.content})
    expect(h.calls.at(-1)?.operation).toBe('open-object')
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
  expect(await h.controller.request({action:'save',editorId:h.editorId,version:1,content})).toMatchObject({ok:true,view:{phase:'ready',dirty:false}})
  expect(h.calls.find(c=>c.operation==='commit-object')?.input).toMatchObject({operationId:'approved_save_operation',changeId:'e'.repeat(64),content})
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
  expect(h.calls.slice(before).map(c=>c.operation)).toEqual(['open-object'])
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
