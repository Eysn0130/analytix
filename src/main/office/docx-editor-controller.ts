import { createHash, randomUUID } from 'node:crypto'
import { extname, isAbsolute, resolve } from 'node:path'
import { docxEditorRequestSchema, type DocxEditorView, type DocxEditorResponse, type DocxEditorCapture } from '../../shared/docx-editor'
import { nativeOfficeCommitInputSchema, nativeOfficeSelectionResponseSchema, nativeOfficeSelectionScopeSchema, type NativeOfficeManualAuthority, type nativeOfficeRecoverySchema } from '../../../packages/runtime/src/contracts/native-office-editing'
import { pluginPackageHostResponseSchema, type PluginPackageHostRequest, type PluginPackageHostResponse, type PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'
import { z } from 'zod'

type Recovery = z.infer<typeof nativeOfficeRecoverySchema>
type Owner = { view:DocxEditorView; binding:PluginPackageView; sessionId:string;
  manual?:NativeOfficeManualAuthority; begin?:{operationId:string;baseRevision:string};
  approved?:{changeId:string;saveOperationId:string;baseRevision:string;expectedBytes?:Uint8Array};
  pending?:{operationId:string;digest:string}; recovery?:Recovery;
  closing?:{sessionId:string;attempted:boolean}; reopening?:{stage:'closing'|'opening';expectedRevision:string};
  capture?:{instanceId:string;captureId:string;target:string;text:string}; decision?:{proposalId:string;operationId:string}; cancel?:string }
const digest = (bytes:Buffer) => createHash('sha256').update(bytes).digest('hex')
class Failure extends Error { constructor(readonly code:NonNullable<DocxEditorResponse['error']>) { super(code) } }
class BeginRefusal extends Failure {}
const fail = (code:NonNullable<DocxEditorResponse['error']>):never => { throw new Failure(code) }
const rangeProjection=z.object({preview:z.object({text:z.string(),truncated:z.boolean()}),
  target:z.object({kind:z.literal('selection'),start:z.unknown(),end:z.unknown()}).passthrough()}).passthrough()
/** Resolve the addressed source with the published engine against Core bytes.
 * Session-only renderer addresses that cannot resolve are deliberately refused. */
export async function verifyDocxCapture(view:DocxEditorView, capture:DocxEditorCapture):Promise<boolean> {
  const target=capture.target
  if (target.kind!=='selection' || !target.start || !target.end || target.coordinateSpace==='tracked') return false
  const {openDocumentRuntime}=await import('@superdoc/docx-engine/document-runtime')
  const runtime=await openDocumentRuntime({bytes:Buffer.from(view.content.data,'base64')})
  try {
    const raw=await runtime.invoke({operationId:'ranges.resolve',input:{
      start:{kind:'point',point:target.start},end:{kind:'point',point:target.end},...(target.story?{in:target.story}:{})}})
    const result=rangeProjection.safeParse(raw)
    return result.success && !result.data.preview.truncated && result.data.preview.text===capture.text &&
      JSON.stringify(result.data.target.start)===JSON.stringify(target.start) && JSON.stringify(result.data.target.end)===JSON.stringify(target.end)
  } finally {await runtime.close()}
}
/** Independently construct only the Core-approved replacement, then ask the
 * published engine to compare the complete exported document. Opaque snapshots
 * pass straight between public SDK operations; application code never reads them. */
export async function replacedDocx(view:DocxEditorView,target:Record<string,unknown>,text:string,expectedText:string) {
  const {openDocumentRuntime}=await import('@superdoc/docx-engine/document-runtime')
  const runtime=await openDocumentRuntime({bytes:Buffer.from(view.content.data,'base64')})
  try {
    const result=await runtime.invoke({operationId:'replace',input:{target,text,expectedText},options:{changeMode:'direct'}})
    if (!z.object({success:z.literal(true)}).safeParse(result).success) throw new Failure('stale_selection')
    const saved=await runtime.save({mode:'review-preserving'})
    if (!saved.bytes.length || saved.bytes.length>16*1024*1024 || saved.warnings.some(warning=>!warning.canProceed)) throw new Failure('unavailable')
    return saved.bytes
  } finally {await runtime.close()}
}
export async function verifyDocxReplacement(expected:Uint8Array,actual:Uint8Array):Promise<boolean> {
  const {openDocumentRuntime}=await import('@superdoc/docx-engine/document-runtime')
  const baseline=await openDocumentRuntime({bytes:expected})
  try {
    const candidate=await openDocumentRuntime({bytes:actual})
    try {
      const targetSnapshot=await candidate.invoke({operationId:'diff.capture'})
      const comparison=await baseline.invoke({operationId:'diff.compare',input:{targetSnapshot}})
      const result=z.object({summary:z.object({hasChanges:z.literal(false)}),
        coverage:z.object({body:z.literal(true),comments:z.literal(true),styles:z.literal(true),numbering:z.literal(true),headerFooters:z.literal(true)}),
        partChanges:z.array(z.unknown()).max(0)}).safeParse(comparison)
      return result.success
    } finally {await candidate.close()}
  } finally {await baseline.close()}
}
/** Ordinary DOCX only. Core owns every path, lease, operation and persistence receipt. */
export function createDocxEditorController(packageHost:(request:PluginPackageHostRequest)=>Promise<PluginPackageHostResponse>, sample?:(view:DocxEditorView)=>Promise<DocxEditorCapture|null>) {
  const owners = new Map<string,Owner>()
  let tail:Promise<unknown> = Promise.resolve(), frozen = false
  const snapshot = (owner?:Owner) => owner ? structuredClone(owner.view) : null
  async function binding(previous?:PluginPackageView) {
    const result = pluginPackageHostResponseSchema.parse(await packageHost({action:'list'}))
    if (!result.ok || !('packages' in result)) return fail('unavailable')
    if (new Set(result.packages.map(p=>p.packageId)).size !== result.packages.length) return fail('unavailable')
    const pkg = result.packages.find(p=>p.packageId === 'analytix-documents')
    if (!pkg?.available || !pkg.materialized || pkg.activationState !== 'recorded' || pkg.desiredState !== 'enabled' ||
      !['open-object','close-object','object-status'].every(op=>pkg.operations.includes(op))) return fail('unavailable')
    if (previous && previous.generationId !== pkg.generationId) return fail('package_changed')
    return pkg
  }
  async function invoke(owner:Pick<Owner,'binding'>, operation:string, input:Record<string,unknown>, closingRetry?:string) {
    owner.binding = await binding(owner.binding)
    if (!owner.binding.operations.includes(operation)) return fail('unavailable')
    const outer = pluginPackageHostResponseSchema.parse(await packageHost({action:'invoke',packageId:owner.binding.packageId,
      generationId:owner.binding.generationId,expectedRevision:owner.binding.activationRevision,contributionId:'workspace-editor',operation,input}))
    if (!outer.ok || !('output' in outer)) return fail('unavailable')
    const result = nativeOfficeSelectionResponseSchema.parse(outer.output)
    // Only a validated, receipt-free native refusal establishes that this
    // begin returned no manual authority. Delivery/persistence ambiguity keeps
    // the exact begin identity for replay.
    if (!result.ok && operation==='begin-manual-edit' && !result.receipt &&
      (result.code==='conflict' || result.code==='forbidden')) throw new BeginRefusal(result.code)
    if (!result.ok && operation==='close-object' && result.code==='session_invalid' && closingRetry && input.sessionId===closingRetry) return result
    if (!result.ok && !(['commit-object','object-status'].includes(operation) && result.receipt)) return fail(result.code === 'forbidden' ? 'forbidden' : result.code === 'conflict' ? 'conflict' : 'unavailable')
    return result
  }
  async function read(owner:Owner, expected?:string) {
    const result = await invoke(owner,'open-object',{object:{workspace:owner.view.workspace,path:owner.view.path}})
    if (!('document' in result)) return fail('unavailable')
    const doc = result.document, bytes = Buffer.from(doc.content,'base64')
    if (bytes.length === 0 || bytes.length > 16*1024*1024 || bytes.toString('base64') !== doc.content ||
      !isAbsolute(doc.path) || resolve(doc.path) !== resolve(owner.view.workspace,owner.view.path) || extname(doc.path).toLowerCase() !== '.docx' ||
      digest(bytes) !== doc.revision || owner.sessionId && doc.sessionId !== owner.sessionId ||
      owner.view.objectId !== '0'.repeat(64) && doc.objectId !== owner.view.objectId) return fail('unavailable')
    if ([...owners.values()].some(previous=>previous!==owner && (previous.sessionId===doc.sessionId || previous.view.objectId===doc.objectId))) return fail('unsaved_changes')
    // The validated new handle must remain available to recovery/explicit discard
    // even when its fresh revision conflicts with the retained editor baseline.
    owner.sessionId = doc.sessionId
    if (expected && expected !== doc.revision) return fail('conflict')
    Object.assign(owner.view,{path:doc.path,objectId:doc.objectId,revision:doc.revision,
      content:{encoding:'base64',kind:'docx',byteLength:bytes.length,sha256:doc.revision,data:doc.content}})
  }
  async function recovery(owner:Owner) {
    const result = await invoke(owner,'object-recovery',{sessionId:owner.sessionId,threadId:owner.view.threadId})
    if (!('recovery' in result)) return fail('unavailable')
    owner.recovery = result.recovery
    return result.recovery
  }
  async function closeHandle(owner:Owner) {
    const closing=owner.closing ??= {sessionId:owner.sessionId,attempted:false}
    if (!closing.sessionId) return fail('unknown')
    const retry=closing.attempted
    closing.attempted=true;owner.view.phase='unknown'
    const result=await invoke(owner,'close-object',{sessionId:closing.sessionId},retry?closing.sessionId:undefined)
    if (!('closed' in result && result.closed) && !(!result.ok && result.code==='session_invalid' && retry)) return fail('unknown')
    owner.sessionId='';delete owner.closing
  }
  async function reopen(owner:Owner) {
    const transition=owner.reopening ??= {stage:'closing',expectedRevision:owner.view.revision}
    owner.view.phase='unknown'
    if (transition.stage==='closing') {
      await closeHandle(owner)
      transition.stage='opening'
      delete owner.capture;delete owner.view.scope;owner.view.proposals=[];owner.view.reviews=[]
    }
    try {await read(owner,transition.expectedRevision)}
    catch(error) {
      if (error instanceof Failure && error.code==='conflict' && owner.sessionId) delete owner.reopening
      throw error
    }
    delete owner.reopening;owner.view.phase='ready'
  }
  async function settle(owner:Owner, result:Awaited<ReturnType<typeof invoke>>) {
    const pending = owner.pending
    if (!pending || !('receipt' in result) || !result.receipt || result.receipt.operationId !== pending.operationId) return fail('unknown')
    const receipt = result.receipt
    if (receipt.status === 'unknown' || receipt.status === 'pending') return fail('unknown')
    if (receipt.status === 'conflict') { delete owner.pending; owner.view.phase='conflict'; return fail('conflict') }
    if (receipt.status !== 'committed' || receipt.revision !== pending.digest) return fail('unknown')
    // Even an exact receipt is historical; only a fresh Core read establishes Saved.
    try {await read(owner,receipt.revision)}
    catch(error) {
      if (error instanceof Failure && error.code==='conflict') {
        delete owner.pending;delete owner.manual;delete owner.begin;delete owner.approved;delete owner.decision
        owner.view.phase='conflict';owner.view.dirty=true
      }
      throw error
    }
    delete owner.pending; delete owner.manual; delete owner.begin; delete owner.approved; delete owner.decision
    owner.view.dirty=false; owner.view.phase='ready'; delete owner.view.scope
    owner.view.proposals=[]; owner.view.reviews=[]; delete owner.capture
    await reopen(owner)
  }
  async function close(owner:Owner, discard:boolean) {
    if (owner.reopening) {
      try {await reopen(owner)}
      catch(error) {if (!(discard && error instanceof Failure && error.code==='conflict' && owner.sessionId)) throw error}
    }
    if (owner.closing) {await closeHandle(owner);owners.delete(owner.view.editorId);return}
    if (owner.pending || owner.begin && !owner.manual || owner.view.phase === 'unknown' && !discard) return fail('unknown')
    if ((owner.view.dirty || owner.approved) && !discard) return fail('unsaved_changes')
    const state = await recovery(owner)
    const change = owner.cancel ?? state.pending?.changeId
    if (change) {
      if (!discard && (owner.view.dirty || !owner.manual && !owner.cancel) || !owner.cancel && (!state.pending || !state.pending.canCancel)) return fail('unknown')
      owner.cancel = change; owner.view.phase='unknown'
      await read(owner)
      const cancelled = await invoke(owner,'cancel-change',{sessionId:owner.sessionId,threadId:owner.view.threadId,changeId:change,baseRevision:owner.view.revision})
      if (!('recovery' in cancelled) || cancelled.recovery.pending?.changeId === change) return fail('unknown')
      delete owner.manual; delete owner.approved; delete owner.cancel
    }
    await closeHandle(owner)
    owners.delete(owner.view.editorId)
  }
  async function run(raw:unknown):Promise<DocxEditorResponse> {
    const parsed = docxEditorRequestSchema.safeParse(raw)
    if (!parsed.success) return {ok:false,view:null,error:'invalid_request'}
    const request = parsed.data
    let owner = 'editorId' in request ? owners.get(request.editorId) : undefined
    try {
      if (request.action === 'open') {
        if (frozen || !isAbsolute(request.workspace) || extname(request.path).toLowerCase() !== '.docx') return fail('invalid_request')
        const workspace=resolve(request.workspace)
        const retained = [...owners.values()].find(o=>o.view.workspace===workspace && o.view.path===resolve(request.workspace,request.path) && o.view.threadId===request.threadId)
        if (retained) return {ok:true,view:snapshot(retained)}
        // Core reuses a principal/object session. Two thread owners must never
        // share and close one another's manual/strict lease through this bridge.
        if ([...owners.values()].some(o=>o.view.workspace===workspace && o.view.path===resolve(workspace,request.path))) return fail('unsaved_changes')
        if (owners.size >= 8) return fail('unavailable')
        owner={binding:await binding(),sessionId:'',view:{editorId:randomUUID(),workspace,path:resolve(workspace,request.path),
          threadId:request.threadId,objectId:'0'.repeat(64),revision:'0'.repeat(64),version:0,phase:'ready',dirty:false,
          content:{encoding:'base64',kind:'docx',byteLength:1,sha256:'0'.repeat(64),data:'AA=='},proposals:[],reviews:[]}}
        await read(owner)
        if ([...owners.values()].some(previous=>previous.sessionId===owner!.sessionId || previous.view.objectId===owner!.view.objectId)) return fail('unsaved_changes')
        owners.set(owner.view.editorId,owner)
        const state=await recovery(owner)
        if (state.pending) { owner.view.phase='unknown'; owner.view.dirty=true }
        return {ok:true,view:snapshot(owner)}
      }
      if (!owner) return fail('unavailable')
      if ((owner.reopening || owner.closing) && !['status','close'].includes(request.action)) return fail('unknown')
      if ('version' in request && request.action !== 'changed' && request.version !== owner.view.version &&
        !(request.action==='applied' && owner.approved && owner.view.phase==='editing' && request.version+1===owner.view.version)) return fail('stale_selection')
      if (frozen && ['begin','changed','capture','decide'].includes(request.action)) return fail('unavailable')
      if (request.action === 'begin') {
        if (owner.cancel) return fail('unknown')
        if (owner.pending || owner.approved || owner.view.phase==='conflict') return fail('unsaved_changes')
        if (!owner.manual) {
          if (!owner.begin) { await reopen(owner); owner.begin={operationId:randomUUID(),baseRevision:owner.view.revision} }
          try {
            const result=await invoke(owner,'begin-manual-edit',{sessionId:owner.sessionId,threadId:owner.view.threadId,...owner.begin})
            if (!('manual' in result) || result.manual.baseRevision !== owner.view.revision) return fail('unknown')
            owner.manual=result.manual
          } catch(error) {
            if (error instanceof BeginRefusal) {delete owner.begin;throw error}
            owner.view.phase='unknown'; return fail('unknown')
          }
        }
        owner.view.phase='editing'
      } else if (request.action === 'changed') {
        if (!owner.manual || owner.pending || owner.view.phase!=='editing' || request.version <= owner.view.version) return fail('stale_selection')
        owner.view.version=request.version; owner.view.dirty=true
      } else if (request.action === 'save') {
        if (owner.pending || owner.cancel) return fail('unknown')
        const authority=owner.manual ?? owner.approved
        if (!authority || owner.view.phase!=='editing') return fail('unsaved_changes')
        const bytes=Buffer.from(request.content.data,'base64'), hash=digest(bytes)
        if (hash !== request.content.sha256 || request.content.kind!=='docx') return fail('invalid_request')
        if (owner.approved && (!owner.approved.expectedBytes || !await verifyDocxReplacement(owner.approved.expectedBytes,bytes))) return fail('forbidden')
        const input=nativeOfficeCommitInputSchema.parse({sessionId:owner.sessionId,threadId:owner.view.threadId,
          operationId:authority.saveOperationId,changeId:authority.changeId,baseRevision:authority.baseRevision,content:request.content})
        owner.pending={operationId:authority.saveOperationId,digest:hash}; owner.view.phase='unknown'
        await settle(owner,await invoke(owner,'commit-object',input))
      } else if (request.action === 'applied') {
        if (!owner.approved || !owner.capture ||
          owner.capture.instanceId!==request.instanceId || owner.capture.captureId!==request.captureId) return fail('stale_selection')
        if (owner.view.phase==='editing' && owner.view.version===request.version+1) return {ok:true,view:snapshot(owner)}
        if (owner.view.phase!=='applying') return fail('stale_selection')
        owner.view.version++; owner.view.phase='editing'
      } else if (request.action === 'status') {
        if (owner.reopening) {await reopen(owner);return {ok:true,view:snapshot(owner)}}
        if (owner.closing) {await closeHandle(owner);owners.delete(owner.view.editorId);return {ok:true,view:null}}
        if (owner.cancel) {await close(owner,true);return {ok:true,view:null}}
        if (owner.approved && !owner.pending) return {ok:true,view:snapshot(owner)}
        if (owner.pending) await settle(owner,await invoke(owner,'object-status',{sessionId:owner.sessionId,operationId:owner.pending.operationId}))
        else if (owner.begin && !owner.manual) return await run({action:'begin',editorId:owner.view.editorId,version:owner.view.version})
        else { const state=await recovery(owner); if (state.pending) return fail('unknown') }
      } else if (request.action === 'close') { await close(owner,request.discard); return {ok:true,view:null} }
      else if (request.action === 'revoke') {
        if (owner.pending || owner.approved || owner.manual) return fail('unsaved_changes')
        await reopen(owner)
      } else if (request.action === 'capture') {
        if (owner.view.phase!=='ready' || owner.manual || owner.pending || owner.view.dirty || owner.approved) return fail('unsaved_changes')
        if (owner.capture) await reopen(owner)
        const observed=await sample?.(snapshot(owner)!)
        if (!observed || observed.editorId!==owner.view.editorId || observed.version!==owner.view.version || observed.revision!==owner.view.revision ||
          observed.instanceId!==request.instanceId || observed.captureId!==request.captureId || !await verifyDocxCapture(owner.view,observed)) return fail('stale_selection')
        const token=randomUUID().replaceAll('-','')
        const result=await invoke(owner,'capture-selection',{sessionId:owner.sessionId,threadId:owner.view.threadId,selectionToken:token,
          changeSequence:owner.view.version,baseRevision:owner.view.revision,text:observed.text,editable:request.editable})
        if (!('scope' in result)) return fail('unavailable')
        const capturedScope=nativeOfficeSelectionScopeSchema.safeParse(result.scope)
        if (!capturedScope.success) return fail('stale_selection')
        const {sessionId,...scope}=capturedScope.data
        if (sessionId!==owner.sessionId || scope.threadId!==owner.view.threadId || scope.selectionToken!==token ||
          scope.baseRevision!==owner.view.revision || scope.changeSequence!==owner.view.version || scope.editable!==request.editable || scope.workbook || scope.presentation) return fail('stale_selection')
        owner.view.scope=scope; owner.capture={instanceId:request.instanceId,captureId:request.captureId,target:JSON.stringify(observed.target),text:observed.text}
      } else if (request.action === 'proposals') {
        const scope=owner.view.scope
        if (!scope || owner.view.phase!=='ready') return fail('stale_selection')
        const checked=await invoke(owner,'selection-read',{sessionId:owner.sessionId,scopeId:scope.scopeId})
        const checkedScope='scope' in checked ? nativeOfficeSelectionScopeSchema.safeParse(checked.scope) : null
        if (!checkedScope?.success || checkedScope.data.selectionToken!==scope.selectionToken || checkedScope.data.baseRevision!==owner.view.revision) return fail('stale_selection')
        const result=await invoke(owner,'proposal-read',{sessionId:owner.sessionId,scopeId:scope.scopeId})
        if (!('proposals' in result)) return fail('unavailable')
        owner.view.proposals=result.proposals; owner.view.reviews=result.localReviews
      } else if (request.action === 'decide') {
        const scope=owner.view.scope, capture=owner.capture
        if (!scope || scope.scopeId!==request.scopeId || !capture || capture.instanceId!==request.instanceId || capture.captureId!==request.captureId ||
          owner.view.phase!=='ready' || owner.view.dirty || owner.manual || owner.pending || request.accept && !scope.editable) return fail('stale_selection')
        const observed=await sample?.(snapshot(owner)!)
        if (!observed || observed.instanceId!==capture.instanceId || observed.captureId!==capture.captureId || observed.editorId!==owner.view.editorId ||
          observed.version!==owner.view.version || observed.revision!==owner.view.revision || JSON.stringify(observed.target)!==capture.target || observed.text!==capture.text || !await verifyDocxCapture(owner.view,observed)) return fail('stale_selection')
        if (owner.decision && owner.decision.proposalId!==request.proposalId) return fail('unknown')
        owner.decision ??= {proposalId:request.proposalId,operationId:randomUUID()}
        const result=await invoke(owner,request.accept?'proposal-accept':'proposal-reject',{sessionId:owner.sessionId,scopeId:scope.scopeId,
          proposalId:request.proposalId,operationId:owner.decision.operationId,...(request.accept?{selectionToken:scope.selectionToken,changeSequence:scope.changeSequence,baseRevision:scope.baseRevision}:{})})
        if (request.accept) {
          if (!('replacement' in result)) return fail('unknown')
          const r=result.replacement
          if (r.proposalId!==request.proposalId || r.operationId!==owner.decision.operationId || r.selectionToken!==scope.selectionToken ||
            r.baseRevision!==owner.view.revision || r.changeSequence!==owner.view.version || r.workbook || r.presentation) return fail('stale_selection')
          owner.approved={changeId:r.changeId,saveOperationId:r.saveOperationId,baseRevision:r.baseRevision}
          owner.view.phase='applying'; owner.view.dirty=true
          owner.approved.expectedBytes=await replacedDocx(owner.view,JSON.parse(capture.target),r.text,capture.text)
          return {ok:true,view:snapshot(owner),replacement:r.text}
        }
        delete owner.decision
      }
      return {ok:true,view:snapshot(owner)}
    } catch(error) {
      const code=error instanceof Failure?error.code:owner?.pending || owner?.closing || owner?.reopening?'unknown':'unavailable'
      if (owner && code==='conflict') owner.view.phase='conflict'
      return {ok:false,view:snapshot(owner),error:code}
    }
  }
  return {
    request(raw:unknown):Promise<DocxEditorResponse> { const next=tail.then(()=>run(raw)); tail=next.catch(()=>undefined); return next },
    views:()=>[...owners.values()].map(o=>snapshot(o)!),
    hasUnsettled:()=>[...owners.values()].some(o=>o.view.dirty || o.manual || o.begin || o.pending || o.approved || o.closing || o.reopening),
    freeze:(value:boolean)=>{frozen=value},
    async destroy() { await tail; for (const owner of owners.values()) { if (owner.view.dirty || owner.pending || owner.closing || owner.reopening) continue; await close(owner,false) } }
  }
}
