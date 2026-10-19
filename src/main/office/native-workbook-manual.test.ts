import {createHash} from 'node:crypto'
import {describe,expect,it,vi} from 'vitest'
import {createNativeOfficeController,type NativeOfficeEngineRequest} from './native-office-controller'
import type {PluginPackageHostRequest,PluginPackageHostResponse,PluginPackageView} from '../../../packages/runtime/src/contracts/plugin-package-host'
import {nativeOfficeResponseSchema} from '../../shared/native-office'
import {logNativeOfficeSaveFailure} from '../logger'

vi.mock('../logger',()=>({logNativeOfficeFailure:vi.fn(),logNativeOfficeManualBeginFailure:vi.fn(),logNativeOfficeSaveFailure:vi.fn()}))
const digest=(bytes:Uint8Array)=>createHash('sha256').update(bytes).digest('hex')
const original=new Uint8Array([80,75,3,4,1]), exported=new Uint8Array([80,75,3,4,2,255,0,128,7]),external=new Uint8Array([80,75,3,4,3,0,254])
const base=digest(original),next=digest(exported),objectId='a'.repeat(64),sessionId='b'.repeat(48),changeId='c'.repeat(64)
const cell=(row:number,value:number)=>({sheet:0,column:0,row,text:String(value),formula:'',value,valueType:'number' as const,numberFormat:0,rowVisible:true as const,columnVisible:true as const,merged:false as const})
const before={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:2,cells:[cell(0,1),cell(1,2),cell(2,0)]}
const after={...before,cells:[...before.cells.slice(0,2),{...cell(2,0),text:'3',formula:'=SUM(A1:A2)',valueType:'formula' as const}]}
const workbook={before,after,results:['1','2','3']}
const bounds={x:0,y:0,width:600,height:400}
type Fault='commit-unknown'|'replace-failed'|'ack-failed'|'export-failed'|'commit-rejected'|'commit-outer'|'commit-invalid-outer'|'commit-invalid-inner'|'commit-conflict'|'readback-conflict'|'reload-failed'
function harness(fault?:Fault,eligible=true,innerCode:'invalid_request'|'not_text'|'too_large'|'persistence_failure'='invalid_request') {
  const calls:NativeOfficeEngineRequest[]=[],hostCalls:PluginPackageHostRequest[]=[]
  let sequence=0,version=base,persistedBytes=original,saved=false,counter=0,prepared=false,destroyed=0,created=0,reloadFailed=false
  let partial=false,scope:Record<string,unknown>|undefined
  const selection=()=>({documentId:objectId,version,changeSequence:sequence,token:'selected_cell_token',kind:'cells',scope:'sheet-range-address-at-version-and-change-sequence',text:'0',
    ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:partial?0:2,endRow:partial?1048575:2}],cells:[cell(partial?0:2,0)],capture:{capturedCharacters:1,totalCharacters:1,truncated:partial,unit:'utf-16',complete:!partial}})
  const change=()=>({version:3,origin:'manual',changeId,threadId:'thread_main',baseRevision:base,revision:saved?next:'',status:saved?'committed':'prepared',workbook,
    saveOperationId:'save_manual_cell',undoOperationId:'undo_manual_cell',canUndo:saved,canCancel:!saved,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''})
  const pkg:PluginPackageView={packageId:'analytix-spreadsheets',packageVersion:'1.0.0',displayName:'Spreadsheets',origin:'development-source',publishable:false,materialized:true,
    generationId:'d'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'e'.repeat(64),available:true,
    operations:['open-object','close-object','object-status','commit-object','object-recovery','plan-workbook-manual','prepare-workbook-manual','capture-selection','selection-read','proposal-read']}
  const controller=createNativeOfficeController({newOperationId:()=>`operation_${++counter}`,onChange:()=>{},
    createSurface:()=>{created++;return {attach:()=>{},hide:()=>{},destroy:()=>{destroyed++},request:async r=>{
      calls.push(r)
      if(r.command==='open'&&saved&&fault==='reload-failed'&&!reloadFailed){reloadFailed=true;throw Error('native reload failed')}
      if(r.command==='open'){sequence=0;version=r.version}
      if(r.command==='replaceCells'){if(fault==='replace-failed')throw Error('typed-mutation-failed');sequence++}
      if(r.command==='export'&&fault==='export-failed')throw Error('export-too-large')
      if(r.command==='ack'&&r.status==='committed'){version=r.persistedVersion!;if(fault==='ack-failed')throw Error('lost native ACK')}
      const acknowledged=r.command==='ack'&&r.status==='committed'
      const state={documentId:objectId,version,kind:'xlsx',changeSequence:sequence,acknowledgedSequence:acknowledged?sequence:0,dirty:!acknowledged&&sequence>0}
      const result={type:'result',channel:'fixture-channel',operationId:r.operationId,documentId:r.documentId,version:r.version,command:r.command,ok:true,state}
      if(r.command==='close')return {...result,state:undefined}
      if(r.command==='captureCells')return {...result,workbook:before}
      if(r.command==='export')return {...result,bytes:exported,exportedSequence:sequence}
      if(r.command==='replaceCells'||r.command==='ack')return result
      return {...result,selection:selection()}
    }}},
    packageHost:async r=>{
      hostCalls.push(r)
      if(r.action==='list')return {ok:true,packages:[pkg]}
      if(r.action!=='invoke')throw Error('unexpected activation')
      let output:unknown
      switch(r.operation){
        case 'open-object':output={ok:true,document:{objectId,sessionId,path:'/workspace/simple.xlsx',revision:digest(persistedBytes),content:Buffer.from(persistedBytes).toString('base64')},workbookEditing:{editable:eligible,...(!eligible?{reason:'unsupported_workbook'}:{})}};break
        case 'object-recovery':output={ok:true,recovery:{current:saved?change():null,pending:prepared&&!saved?change():null}};break
        case 'capture-selection':{const {text,...input}=r.input;scope={...input,scopeId:'f'.repeat(48),parts:[{kind:'literal',text}]};output={ok:true,scope};break}
        case 'selection-read':output={ok:true,scope};break
        case 'proposal-read':output={ok:true,proposals:[],localReviews:[]};break
        case 'plan-workbook-manual':output={ok:true,dependencyBounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}};break
        case 'prepare-workbook-manual':prepared=true;output={ok:true,workbookManual:{version:3,origin:'manual',changeId,baseRevision:base,saveOperationId:'save_manual_cell',undoOperationId:'undo_manual_cell',workbook}};break
        case 'commit-object':
          if(fault==='commit-unknown')throw Error('lost reply')
          if(fault==='commit-outer')return {ok:false,code:'adapter_unavailable',message:'PRIVATE_COMMIT_MESSAGE_CANARY'}
          if(fault==='commit-invalid-outer')return {ok:false,code:'PRIVATE_OUTER_CODE_CANARY',message:'PRIVATE_COMMIT_MESSAGE_CANARY'} as unknown as PluginPackageHostResponse
          if(fault==='commit-invalid-inner'){output={ok:false,code:'PRIVATE_INNER_CODE_CANARY',message:'PRIVATE_COMMIT_MESSAGE_CANARY'};break}
          if(fault==='commit-rejected'){output={ok:false,code:innerCode,message:'PRIVATE_COMMIT_MESSAGE_CANARY'};break}
          if(fault==='commit-conflict'){output={ok:true,receipt:{operationId:'save_manual_cell',revision:'',status:'conflict',savedAt:''}};break}
          saved=true;persistedBytes=fault==='readback-conflict'?external:exported;output={ok:true,receipt:{operationId:'save_manual_cell',revision:next,status:'committed',savedAt:'2026-10-19T00:00:00Z'}};break
        case 'object-status':output={ok:true,receipt:{operationId:'save_manual_cell',revision:'',status:'unknown',savedAt:''}};break
        case 'close-object':output={ok:true,closed:true};break
        default:throw Error('unexpected operation '+r.operation)
      }
      return {ok:true,output}
    }})
  const open=()=>controller.request({action:'open',workspace:'/workspace',path:'simple.xlsx',threadId:'thread_main',bounds})
  const edit=()=>controller.request({action:'editWorkbookCell',objectId,revision:base,expectedChangeSequence:0,threadId:'thread_main',selectionToken:'selected_cell_token',edit:{type:'formula',formula:'=SUM(A1:A2)'}})
  return {controller,calls,hostCalls,open,edit,setPartial:()=>{partial=true},get destroyed(){return destroyed},get created(){return created}}
}
describe('protected local workbook manual controller',()=>{
  it('separates Core dependency reads from one native target and requires the committed receipt',async()=>{
    const h=harness();expect((await h.open()).ok).toBe(true)
    const result=await h.edit();expect(result.ok).toBe(true);expect(nativeOfficeResponseSchema.safeParse(result).success).toBe(true)
    expect(result.view?.revision).toBe(next);expect(result.view?.recovery?.current).toMatchObject({version:3,origin:'manual',status:'committed',changeId})
    expect(result.view).toMatchObject({status:'ready',dirty:false,editing:false,saving:false,canUndo:true,changeSequence:0,acknowledgedSequence:0})
    expect(h.created).toBe(2);expect(h.destroyed).toBe(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='open-object')).toHaveLength(2)
    expect(h.calls.filter(r=>r.command==='open')).toEqual([expect.objectContaining({version:base,bytes:original}),expect.objectContaining({version:next,bytes:exported})])
    const ops=h.hostCalls.flatMap(r=>r.action==='invoke'?[r.operation]:[])
    expect(ops).toContain('prepare-workbook-manual');expect(ops.some(op=>op.startsWith('model-')||op.startsWith('proposal-'))).toBe(false)
    const read=h.calls.find(r=>r.command==='captureCells'),write=h.calls.find(r=>r.command==='replaceCells')
    expect(read?.bounds).toEqual({sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2})
    expect(write?.writeTarget).toEqual({sheet:0,column:0,row:2});expect(h.calls.some(r=>r.command==='manual-edit')).toBe(false)
    expect(write?.candidate).toEqual({changeId,saveOperationId:'save_manual_cell',baseRevision:base})
    const prepare=h.hostCalls.find(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')
    expect(prepare&&'input'in prepare&&prepare.input.writeTarget).toEqual({sheet:0,sheetName:'Sheet1',column:0,row:2})
  })
  it('keeps complex input readonly before any typed edit or commit',async()=>{
    const h=harness(undefined,false);await h.open();expect((await h.edit()).error).toBe('unsupported_selection')
    expect(h.calls.some(r=>['edit','replaceCells','export'].includes(r.command))).toBe(false)
  })
  it('never repeats a commit after its reply is lost',async()=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness('commit-unknown');await h.open();expect((await h.edit()).error).toBe('unknown')
    expect(logNativeOfficeSaveFailure).toHaveBeenLastCalledWith('unknown',{phase:'commit',outerSchemaAccepted:null,innerSchemaAccepted:null,outerCode:null,innerCode:null,receiptPresent:null,receiptStatus:null})
    await h.controller.request({action:'saveStatus',objectId})
    expect(logNativeOfficeSaveFailure).toHaveBeenLastCalledWith('unknown',{phase:'query',outerSchemaAccepted:true,innerSchemaAccepted:true,outerCode:null,innerCode:null,receiptPresent:true,receiptStatus:'unknown'})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.hostCalls.some(r=>r.action==='invoke'&&r.operation==='object-status')).toBe(true)
  })
  it('retires a partially failed native writer without export',async()=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness('replace-failed');await h.open();expect((await h.edit()).error).toBe('save_failed')
    expect(h.destroyed).toBeGreaterThan(0);expect(h.calls.some(r=>r.command==='export')).toBe(false)
    expect(h.controller.getView()?.recovery?.pending).toMatchObject({version:3,status:'prepared'})
    expect(logNativeOfficeSaveFailure).toHaveBeenCalledExactlyOnceWith('engine_unavailable',{phase:'replace',outerSchemaAccepted:null,innerSchemaAccepted:null,outerCode:null,innerCode:null,receiptPresent:null,receiptStatus:null})
  })
  it.each(['invalid_request','not_text','too_large','persistence_failure'] as const)('retains Core %s diagnostics while preserving save_failed and the original revision',async code=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness('commit-rejected',true,code);await h.open()
    expect(await h.edit()).toMatchObject({ok:false,error:'save_failed',view:{revision:base,dirty:true}})
    expect(logNativeOfficeSaveFailure).toHaveBeenCalledExactlyOnceWith('save_failed',{phase:'commit',outerSchemaAccepted:true,innerSchemaAccepted:true,outerCode:null,innerCode:code,receiptPresent:false,receiptStatus:null})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.calls.filter(r=>r.command==='ack')).toEqual([expect.objectContaining({status:'failed'})])
    expect(JSON.stringify(vi.mocked(logNativeOfficeSaveFailure).mock.calls)).not.toContain('PRIVATE_COMMIT_MESSAGE_CANARY')
  })
  it.each([
    ['commit-outer',true,null,'adapter_unavailable'],
    ['commit-invalid-outer',false,null,null],
    ['commit-invalid-inner',true,false,null]
  ] as const)('records %s schema evidence without logging an envelope or changing unknown',async(fault,outerSchemaAccepted,innerSchemaAccepted,outerCode)=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness(fault);await h.open();expect((await h.edit()).error).toBe('unknown')
    expect(logNativeOfficeSaveFailure).toHaveBeenCalledExactlyOnceWith('unknown',{phase:'commit',outerSchemaAccepted,innerSchemaAccepted,outerCode,innerCode:null,receiptPresent:null,receiptStatus:null})
    expect(JSON.stringify(vi.mocked(logNativeOfficeSaveFailure).mock.calls)).not.toContain('PRIVATE_')
  })
  it('keeps export refusal distinct from a Core commit',async()=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness('export-failed');await h.open();expect((await h.edit()).error).toBe('save_failed')
    expect(h.hostCalls.some(r=>r.action==='invoke'&&r.operation==='commit-object')).toBe(false)
    expect(logNativeOfficeSaveFailure).toHaveBeenCalledExactlyOnceWith('save_failed',{phase:'export',outerSchemaAccepted:null,innerSchemaAccepted:null,outerCode:null,innerCode:null,receiptPresent:null,receiptStatus:null})
  })
  it('records a Core conflict as commit after its native ACK succeeds',async()=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness('commit-conflict');await h.open();expect((await h.edit()).error).toBe('conflict')
    expect(logNativeOfficeSaveFailure).toHaveBeenCalledExactlyOnceWith('conflict',{phase:'commit',outerSchemaAccepted:true,innerSchemaAccepted:true,outerCode:null,innerCode:null,receiptPresent:true,receiptStatus:'conflict'})
  })
  it('reloads exact Core bytes after a committed manual save loses native ACK',async()=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness('ack-failed');await h.open();const result=await h.edit()
    expect(result.ok).toBe(true);expect(result.view?.revision).toBe(next);expect(result.view?.dirty).toBe(false)
    expect(result.view?.canUndo).toBe(true);expect(h.destroyed).toBeGreaterThan(0)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.calls.filter(r=>r.command==='open')).toHaveLength(2)
    expect(h.calls.filter(r=>r.command==='open')[1]).toMatchObject({version:next,bytes:exported})
    expect(result.view).toMatchObject({status:'ready',editing:false,saving:false,changeSequence:0,acknowledgedSequence:0})
    expect(logNativeOfficeSaveFailure).toHaveBeenCalledExactlyOnceWith('engine_unavailable',{phase:'ack',outerSchemaAccepted:true,innerSchemaAccepted:true,outerCode:null,innerCode:null,receiptPresent:true,receiptStatus:'committed'})
  })
  it('rejects a changed Core readback without loading the external bytes into the native view',async()=>{
    const h=harness('readback-conflict');await h.open();const result=await h.edit()
    expect(result).toMatchObject({ok:false,error:'conflict',view:{saving:false,editing:false,canUndo:false}})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='open-object')).toHaveLength(2)
    expect(h.calls.filter(r=>r.command==='open')).toEqual([expect.objectContaining({version:base,bytes:original})])
    expect(h.created).toBe(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.calls.filter(r=>r.command==='export')).toHaveLength(1)
  })
  it('keeps a failed committed reload unknown until saveStatus rereads and reloads without another write',async()=>{
    const h=harness('reload-failed');await h.open();const result=await h.edit()
    expect(result).toMatchObject({ok:false,error:'unknown',view:{saving:true,canUndo:false}})
    expect(result.view?.status).not.toBe('ready')
    expect(h.calls.filter(r=>r.command==='open')).toHaveLength(2)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='open-object')).toHaveLength(2)
    const query=await h.controller.request({action:'saveStatus',objectId})
    expect(query).toMatchObject({ok:true,view:{revision:next,status:'ready',dirty:false,editing:false,saving:false,canUndo:true,changeSequence:0,acknowledgedSequence:0}})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='open-object')).toHaveLength(3)
    expect(h.calls.filter(r=>r.command==='open')).toEqual([expect.objectContaining({version:base,bytes:original}),expect.objectContaining({version:next,bytes:exported}),expect.objectContaining({version:next,bytes:exported})])
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.calls.filter(r=>r.command==='export')).toHaveLength(1)
    expect(h.calls.filter(r=>r.command==='ack')).toHaveLength(1)
    expect(h.hostCalls.some(r=>r.action==='invoke'&&r.operation==='object-status')).toBe(false)
  })
  it('projects actual full-range and partial evidence into the Core readonly scope',async()=>{
    const h=harness();h.setPartial();await h.open()
    const result=await h.controller.request({action:'reference',objectId,revision:base,expectedChangeSequence:0,threadId:'thread_main',selectionToken:'selected_cell_token',editable:false})
    expect(result.ok).toBe(true)
    const projected=result.view?.scope?.parts.filter(p=>p.kind==='literal').map(p=>p.text).join('')
    expect(projected).toContain('"Sheet1"!A1:A1048576');expect(projected).toContain('partial');expect(projected).toContain('do not infer whole-range totals')
    expect(result.view?.scope?.editable).toBe(false);expect(result.view?.scope?.workbook).toBeUndefined()
    expect(h.calls.some(r=>['edit','replaceCells','export'].includes(r.command))).toBe(false)
  })
})
