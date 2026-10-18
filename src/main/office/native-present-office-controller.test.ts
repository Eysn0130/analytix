import {createHash} from 'node:crypto'
import {expect,it,vi} from 'vitest'
import {createNativeOfficeController,type NativeOfficeEngineRequest} from './native-office-controller'
import {nativeOfficeRequestSchema,nativeOfficeResponseSchema} from '../../shared/native-office'
import type {PresentOfficeSnapshot} from '../ipc/tool-result-local-display-ipc'
import type {PluginPackageHostRequest,PluginPackageView} from '../../../packages/runtime/src/contracts/plugin-package-host'

vi.mock('../logger', () => ({ logNativeOfficeFailure: vi.fn() }))
import {logNativeOfficeFailure} from '../logger'

const selector = {threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
const bounds={x:0,y:0,width:800,height:600}
function cap(kind:'docx'|'pptx'='docx') {
  const owner=new AbortController(),bytes=Buffer.from([80,75,3,4,0,1]),digest=createHash('sha256').update(bytes).digest('hex')
  let current=true
  const snapshot:PresentOfficeSnapshot={path:'/workspace/report.'+kind,workspaceRoot:'/workspace',kind,selector,bytes,digest,
    size:bytes.length,mtimeMs:1,truncated:false,signal:owner.signal,isCurrent:() => current,guard:vi.fn(async () => current)}
  return {snapshot,owner,change:() => {current=false}}
}
function harness() {
  let unavailable=false,operation=0
  const hostCalls:PluginPackageHostRequest[]=[],surfaces:Array<{calls:NativeOfficeEngineRequest[];emit:(e:unknown)=>void;hide:ReturnType<typeof vi.fn>;destroy:ReturnType<typeof vi.fn>;protectedPresent?:boolean;reply:(r:NativeOfficeEngineRequest)=>Promise<unknown>}> = []
  const pkg:PluginPackageView={packageId:'analytix-documents',packageVersion:'1.0.0',displayName:'Documents',origin:'development-source',publishable:false,materialized:true,
    generationId:'a'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'b'.repeat(64),available:true,
    operations:['open-object','close-object','commit-object','object-status']}
  const original=cap(),ordinaryId='d'.repeat(64)
  const controller=createNativeOfficeController({newOperationId:()=>'operation_'+ ++operation,onChange() {},
    packageHost:async request=> {
      hostCalls.push(request)
      if (request.action==='list') return {ok:true,packages:unavailable?[]:[pkg,{...pkg,packageId:'analytix-presentations'}]}
      if (request.action==='invoke' && request.operation==='open-object') return {ok:true,output:{ok:true,document:{objectId:ordinaryId,sessionId:'e'.repeat(48),path:original.snapshot.path,revision:original.snapshot.digest,content:original.snapshot.bytes.toString('base64')}}}
      if (request.action==='invoke' && request.operation==='close-object') return {ok:true,output:{ok:true,closed:true}}
      throw Error('unexpected protected Core invoke')
    },createSurface:(emit,protectedPresent)=>{
      let kind: 'docx' | 'pptx' = 'docx'
      const surface={emit,protectedPresent,calls:[] as NativeOfficeEngineRequest[],attach:vi.fn(),hide:vi.fn(),destroy:vi.fn(),reply:async(request:NativeOfficeEngineRequest):Promise<unknown> => {
        const base={type:'result',command:request.command,operationId:request.operationId,documentId:request.documentId,version:request.version,ok:true}
        if(request.command==='close')return base
        if(request.command==='open')kind=request.kind as 'docx' | 'pptx'
        const state={documentId:request.documentId,version:request.version,kind,changeSequence:0,acknowledgedSequence:0,dirty:false}
        return request.command==='local-view'?{...base,state,view:{zoom:125,zoomType:3,fit:false,page:kind==='pptx'?1:null,pages:kind==='pptx'?3:null,readOnly:true,modified:false,
          ...(kind==='pptx'?{playback:{generation:0,sequence:1,phase:'idle',slideIndex:null,slideCount:null}}:{})}}:
          {...base,state,selection:{documentId:request.documentId,version:request.version,changeSequence:0,kind:'unavailable',scope:'engine selection interface unavailable in this view'}}
      },request:async(request:NativeOfficeEngineRequest)=>{surface.calls.push(request);return surface.reply(request)}}
      surfaces.push(surface);return surface
    }})
  return {controller,surfaces,hostCalls,unavailable:()=>{unavailable=true},ordinaryId,original}
}
it.each(['docx','pptx'] as const)('opens %s selector-only cap in an independent readonly native instance',async kind=>{
  const h=harness(),c=cap(kind),request={action:'openPresent' as const,protectedPresent:selector,bounds}
  expect(nativeOfficeRequestSchema.safeParse({...request,path:'/forged'}).success).toBe(false)
  expect(await h.controller.request(request)).toMatchObject({ok:false,error:'invalid_request'})
  const result=await h.controller.openPresent(request,c.snapshot)
  expect(result).toMatchObject({ok:true,view:{protectedPresent:selector,revision:c.snapshot.digest,status:'ready',dirty:false}})
  expect(nativeOfficeResponseSchema.safeParse(result).success).toBe(true)
  expect(h.hostCalls.every(r=>r.action==='list')).toBe(true);expect(h.surfaces[0].protectedPresent).toBe(true)
  expect(h.surfaces[0].calls[0]).toMatchObject({command:'open',kind,bytes:c.snapshot.bytes})
  expect(result.view?.objectId).not.toBe(h.ordinaryId);expect(result.view?.selection).toBeUndefined()
  const target={objectId:result.view!.objectId,revision:result.view!.revision,expectedChangeSequence:0}
  for(const request of [{action:'captureSelection'},{action:'annotate',...target},{action:'save',...target},{action:'annotationSave',objectId:target.objectId,threadId:'thread-a',note:'blocked',sourceRevision:target.revision}]) expect(await h.controller.request(request)).toMatchObject({ok:false})
  expect(h.surfaces[0].calls).toHaveLength(1);expect(h.controller.hasPendingAnnotations()).toBe(false)
  await h.controller.request({action:'close',objectId:target.objectId});expect(h.surfaces[0].destroy).toHaveBeenCalledOnce()
  expect(h.hostCalls.every(r=>r.action==='list')).toBe(true)
})
it('retiring a Present cap preserves same-file ordinary dirty state and unsaved notes',async()=>{
  const h=harness();await h.controller.request({action:'open',workspace:'/workspace',path:'report.docx',bounds,threadId:'thread-a'})
  await h.controller.request({action:'annotate',objectId:h.ordinaryId,revision:h.original.snapshot.digest,expectedChangeSequence:0})
  const opened=h.surfaces[0].calls[0]
  h.surfaces[0].emit({type:'changed',documentId:h.ordinaryId,version:opened.version,operationId:opened.operationId,state:{documentId:h.ordinaryId,version:opened.version,kind:'docx',changeSequence:1,acknowledgedSequence:0,dirty:true}})
  await h.controller.request({action:'annotationSave',objectId:h.ordinaryId,threadId:'thread-a',note:'unsaved ordinary note',sourceRevision:opened.version})
  expect(h.controller.hasPendingAnnotations()).toBe(true)
  const original=h.controller.getViews()[0],c=cap();expect((await h.controller.openPresent({action:'openPresent',protectedPresent:selector,bounds},c.snapshot)).ok).toBe(true)
  c.owner.abort();await vi.waitFor(()=>expect(h.surfaces[1].destroy).toHaveBeenCalledOnce())
  expect(h.controller.getView()).toBeNull();expect(h.controller.getViews()).toEqual([original]);expect(original.dirty).toBe(true)
  expect(h.controller.hasPendingAnnotations()).toBe(true);expect(h.surfaces[0].destroy).not.toHaveBeenCalled()
})
it('reports a failed new Present as unavailable without returning an old ordinary view',async()=>{
  const h=harness();await h.controller.request({action:'open',workspace:'/workspace',path:'report.docx',bounds})
  h.unavailable();const c=cap()
  expect(await h.controller.openPresent({action:'openPresent',protectedPresent:selector,bounds},c.snapshot)).toEqual({ok:false,view:null,error:'package_unavailable'})
  expect(logNativeOfficeFailure).toHaveBeenLastCalledWith('controller_package','package_unavailable',true)
  expect(h.controller.getViews()[0].objectId).toBe(h.ordinaryId)
})
it('permanently retires a file-version-lost cap and ignores its late events after a successor',async()=>{
  const h=harness(),first=cap();const opened=await h.controller.openPresent({action:'openPresent',protectedPresent:selector,bounds},first.snapshot)
  first.change();expect(await h.controller.request({action:'viewControl',objectId:opened.view!.objectId,revision:opened.view!.revision,expectedChangeSequence:0,control:'read'})).toMatchObject({ok:false,view:null})
  const next=cap();expect((await h.controller.openPresent({action:'openPresent',protectedPresent:selector,bounds},next.snapshot)).ok).toBe(true)
  const before=h.controller.getView();h.surfaces[0].emit({type:'fatal',code:'office-protocol-invalid'});first.owner.abort()
  expect(h.controller.getView()).toEqual(before);expect(h.surfaces[1].destroy).not.toHaveBeenCalled()
})

it('a view-control event from an earlier visibility epoch cannot republish its view',async()=>{
  const h=harness(),c=cap(),result=await h.controller.openPresent({action:'openPresent',protectedPresent:selector,bounds},c.snapshot)
  const surface=h.surfaces[0],reply=surface.reply,opened=surface.calls[0]
  let finish!:(value:unknown)=>void,controlRequest!:NativeOfficeEngineRequest
  surface.reply=request=>request.command==='local-view'?new Promise(resolve=>{finish=resolve;controlRequest=request}):reply(request)
  const pending=h.controller.request({action:'viewControl',objectId:result.view!.objectId,revision:result.view!.revision,expectedChangeSequence:0,control:'fit'})
  await vi.waitFor(()=>expect(finish).toBeDefined());await h.controller.request({action:'hide'})
  const rebound=h.controller.request({action:'bounds',bounds})
  surface.emit({type:'view',documentId:opened.documentId,version:opened.version,operationId:opened.operationId,view:{zoom:200,zoomType:3,fit:false,page:null,pages:null,readOnly:true,modified:false}})
  await new Promise(resolve=>setImmediate(resolve));expect(h.controller.getView()?.previewView).toBeUndefined()
  finish(await reply(controlRequest));expect(await pending).toMatchObject({ok:false,error:'stale_selection'})
  expect((await rebound).ok).toBe(true);expect(h.controller.getView()?.previewView).toBeUndefined()
})


it.each(['guard_before','guard_after','guard_final','current_final'] as const)('reports the first lost open checkpoint %s without reviving its preview',async checkpoint=>{
  vi.mocked(logNativeOfficeFailure).mockClear()
  const h=harness(),c=cap()
  const guard=vi.mocked(c.snapshot.guard)
  if(checkpoint==='guard_before') {
    // Admission, create, initial ownership and attach remain valid; the guard
    // immediately before dispatch rejects this same owner.
    guard.mockResolvedValueOnce(true).mockResolvedValueOnce(true).mockResolvedValueOnce(true).mockResolvedValueOnce(true).mockResolvedValue(false)
  } else {
    let hooked=false
    guard.mockImplementation(async()=>{
      const surface=h.surfaces[0]
      if(surface && !hooked) {
        hooked=true
        const reply=surface.reply
        surface.reply=async request=>{
          const result=await reply(request)
          if(checkpoint==='guard_after') guard.mockResolvedValue(false)
          else {
            let readsAfterOpen=0
            guard.mockImplementation(async()=>{
              readsAfterOpen++
              if(checkpoint==='guard_final') return readsAfterOpen===1
              if(readsAfterOpen===2) void h.controller.request({action:'hide'})
              return true
            })
          }
          return result
        }
      }
      return true
    })
  }
  const result=await h.controller.openPresent({action:'openPresent',protectedPresent:selector,bounds},c.snapshot)
  expect(result).toEqual({ok:false,view:null,error:'unavailable'})
  expect(h.controller.getView()).toBeNull()
  expect(h.surfaces[0].destroy).toHaveBeenCalledOnce()
  expect(h.surfaces[0].calls).toHaveLength(checkpoint==='guard_before'?0:1)
  const state={openDispatched:checkpoint!=='guard_before',openResolved:checkpoint!=='guard_before',guardPassed:checkpoint==='current_final',
    owns:true,visible:checkpoint!=='current_final',epochMatch:checkpoint!=='current_final',snapshotCurrent:true,ownerAborted:false}
  expect(logNativeOfficeFailure).toHaveBeenCalledExactlyOnceWith('controller_surface_open_'+checkpoint,'unavailable',true,state)
  expect(JSON.stringify(vi.mocked(logNativeOfficeFailure).mock.calls)).not.toContain('/workspace/report.docx')
})
