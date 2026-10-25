import {createHash} from 'node:crypto'
import {describe,expect,it,vi} from 'vitest'
import {createNativeOfficeController,type NativeOfficeEngineRequest} from './native-office-controller'
import type {PluginPackageHostRequest,PluginPackageHostResponse,PluginPackageView} from '../../../packages/runtime/src/contracts/plugin-package-host'
import type {NativeWorkbookCandidate} from '../../../packages/runtime/src/contracts/native-office-editing'
import {nativeOfficeResponseSchema} from '../../shared/native-office'
import {logNativeOfficeSaveFailure} from '../logger'

vi.mock('../logger',()=>({logNativeOfficeFailure:vi.fn(),logNativeOfficeManualBeginFailure:vi.fn(),logNativeOfficeSaveFailure:vi.fn()}))
const digest=(bytes:Uint8Array)=>createHash('sha256').update(bytes).digest('hex')
// Transport fixtures only: Main verifies the Core envelope; package fidelity is
// covered by Core codec tests. These bytes do not claim a native XLSX export.
const original=new Uint8Array([80,75,3,4,1]),candidateBytes=new Uint8Array([80,75,3,4,2,255,0,128,7]),external=new Uint8Array([80,75,3,4,3,0,254])
const base=digest(original),next=digest(candidateBytes),objectId='a'.repeat(64),sessionId='b'.repeat(48),changeId='c'.repeat(64)
const cell=(row:number,value:number)=>({sheet:0,column:0,row,text:String(value),formula:'',value,valueType:'number' as const,numberFormat:0,rowVisible:true as const,columnVisible:true as const,merged:false as const})
const before={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:2,cells:[cell(0,1),cell(1,2),cell(2,0)]}
const after={...before,cells:[...before.cells.slice(0,2),{...cell(2,0),text:'3',formula:'=SUM(A1:A2)',valueType:'formula' as const}]}
const workbook={before,after,results:['1','2','3']}
const bounds={x:0,y:0,width:600,height:400}
type Fault='prepare-lost'|'candidate-failed'|'candidate-hash'|'candidate-length'|'candidate-change'|'candidate-save'|'candidate-base'|'candidate-kind'|
  'prepare-rejected'|'prepare-rejected-old-current'|'prepare-rejected-undone-current'|'prepare-rejected-new-current'|'prepare-rejected-query-failed'|'prepare-lost-empty'|'candidate-hash-current'|
  'late-capture-changed'|'late-selection-changed'|'commit-unknown'|'commit-lost-committed'|'commit-rejected'|'commit-outer'|
  'commit-invalid-outer'|'commit-invalid-inner'|'commit-conflict'|'readback-conflict'|'reload-failed'
function harness(fault?:Fault,eligible=true,innerCode:'invalid_request'|'not_text'|'too_large'|'persistence_failure'='invalid_request') {
  const calls:NativeOfficeEngineRequest[]=[],hostCalls:PluginPackageHostRequest[]=[]
  let sequence=0,version=base,persistedBytes=original,saved=false,counter=0,prepared=false,destroyed=0,created=0,reloadFailed=false,captures=0,prepareAttempts=0
  let partial=false,scope:Record<string,unknown>|undefined
  let preparedWorkbook=workbook
  const selection=()=>({documentId:objectId,version,changeSequence:sequence,token:'selected_cell_token',kind:'cells',scope:'sheet-range-address-at-version-and-change-sequence',text:'0',
    ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:partial?0:2,endRow:partial?1048575:2}],cells:[cell(partial?0:2,0)],capture:{capturedCharacters:1,totalCharacters:1,truncated:partial,unit:'utf-16',complete:!partial}})
  const change=()=>({version:3,origin:'manual',changeId,threadId:'thread_main',baseRevision:base,revision:saved?next:'',status:saved?'committed':'prepared',workbook:preparedWorkbook,
    saveOperationId:'save_manual_cell',undoOperationId:'undo_manual_cell',canUndo:saved,canCancel:!saved,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''})
  const candidate=():unknown=>{
    const value:NativeWorkbookCandidate={changeId,saveOperationId:'save_manual_cell',baseRevision:base,
      content:{encoding:'base64',kind:'xlsx',byteLength:candidateBytes.byteLength,sha256:next,data:Buffer.from(candidateBytes).toString('base64')}}
    switch(fault){
      case 'candidate-hash':case 'candidate-hash-current':return {...value,content:{...value.content,sha256:'f'.repeat(64)}}
      case 'candidate-length':return {...value,content:{...value.content,byteLength:candidateBytes.byteLength+1}}
      case 'candidate-change':return {...value,changeId:'f'.repeat(64)}
      case 'candidate-save':return {...value,saveOperationId:'save_another_cell'}
      case 'candidate-base':return {...value,baseRevision:'f'.repeat(64)}
      case 'candidate-kind':return {...value,content:{...value.content,kind:'docx'}}
      default:return value
    }
  }
  const pkg:PluginPackageView={packageId:'analytix-spreadsheets',packageVersion:'1.0.0',displayName:'Spreadsheets',origin:'development-source',publishable:false,materialized:true,
    generationId:'d'.repeat(64),activationState:'recorded',desiredState:'enabled',activationRevision:1,activationId:'e'.repeat(64),available:true,
    operations:['open-object','close-object','object-status','commit-object','object-recovery','cancel-change','plan-workbook-manual','prepare-workbook-manual','capture-selection','selection-read','proposal-read']}
  const controller=createNativeOfficeController({newOperationId:()=>`operation_${++counter}`,onChange:()=>{},
    createSurface:()=>{created++;return {attach:()=>{},hide:()=>{},destroy:()=>{destroyed++},request:async r=>{
      calls.push(r)
      if(r.command==='open'&&saved&&fault==='reload-failed'&&!reloadFailed){reloadFailed=true;throw Error('native reload failed')}
      if(r.command==='open'){sequence=0;version=r.version}
      if(['replaceCells','export','ack','manual-edit'].includes(r.command))throw Error('XLSX must remain a readonly native capture surface')
      const state={documentId:objectId,version,kind:'xlsx',changeSequence:sequence,acknowledgedSequence:0,dirty:false}
      const result={type:'result',channel:'fixture-channel',operationId:r.operationId,documentId:r.documentId,version:r.version,command:r.command,ok:true,state}
      if(r.command==='close')return {...result,state:undefined}
      if(r.command==='captureCells'){
        captures++
        if(captures===2&&fault==='late-selection-changed')throw Error('stale-selection')
        return {...result,workbook:captures===2&&fault==='late-capture-changed'?{...before,cells:[cell(0,99),...before.cells.slice(1)]}:structuredClone(before)}
      }
      return {...result,selection:selection()}
    }}},
    packageHost:async r=>{
      hostCalls.push(r)
      if(r.action==='list')return {ok:true,packages:[pkg]}
      if(r.action!=='invoke')throw Error('unexpected activation')
      let output:unknown
      switch(r.operation){
        case 'open-object':output={ok:true,document:{objectId,sessionId,path:'/workspace/simple.xlsx',revision:digest(persistedBytes),content:Buffer.from(persistedBytes).toString('base64')},workbookEditing:{editable:eligible,...(!eligible?{reason:'unsupported_workbook'}:{})}};break
        case 'object-recovery':
          if(prepareAttempts>0&&fault==='prepare-rejected-query-failed')throw Error('lost recovery reply')
          if(!saved&&['prepare-rejected-old-current','prepare-rejected-undone-current'].includes(fault??'')){
            const undone=fault==='prepare-rejected-undone-current'
            output={ok:true,recovery:{current:{...change(),changeId:'8'.repeat(64),baseRevision:undone?base:'9'.repeat(64),revision:base,status:undone?'undone':'committed',
              saveOperationId:'save_previous_cell',undoOperationId:'undo_previous_cell',canUndo:!undone,canCancel:false},pending:null}};break
          }
          if(prepareAttempts>0&&fault==='prepare-rejected-new-current'){
            output={ok:true,recovery:{current:{...change(),changeId:'8'.repeat(64),revision:'',status:'unknown',canUndo:false,canCancel:false},pending:null}};break
          }
          // A fault response for this attempt must not be mistaken for proof
          // that its durable approval is absent, even when pending is null.
          if(prepareAttempts>0&&fault==='candidate-hash-current'){
            output={ok:true,recovery:{current:{...change(),revision:next,status:'committed',canUndo:true,canCancel:false},pending:null}};break
          }
          output={ok:true,recovery:{current:saved?change():null,pending:prepared&&!saved?change():null}};break
        case 'capture-selection':{const {text,...input}=r.input;scope={...input,scopeId:'f'.repeat(48),parts:[{kind:'literal',text}]};output={ok:true,scope};break}
        case 'selection-read':output={ok:true,scope};break
        case 'proposal-read':output={ok:true,proposals:[],localReviews:[]};break
        case 'plan-workbook-manual':output={ok:true,dependencyBounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}};break
        case 'prepare-workbook-manual':
          prepared=true;prepareAttempts++
          if(prepareAttempts===1&&['prepare-rejected','prepare-rejected-old-current','prepare-rejected-undone-current','prepare-rejected-new-current','prepare-rejected-query-failed'].includes(fault??'')){
            prepared=false;output={ok:false,code:'conflict',message:'Preparation was explicitly rejected.'};break
          }
          if(fault==='prepare-lost-empty'&&prepareAttempts===1){prepared=false;throw Error('lost prepare reply with no discovered reservation')}
          if(fault==='prepare-lost'&&prepareAttempts===1)throw Error('lost prepare reply')
          if(fault==='candidate-failed'&&prepareAttempts===1){output={ok:false,code:'persistence_failure',message:'PRIVATE_CANDIDATE_MESSAGE_CANARY'};break}
          if((r.input.edit as {type:string}).type==='number'){
            const value=(r.input.edit as {value:number}).value
            preparedWorkbook={before,after:{...before,cells:[...before.cells.slice(0,2),cell(2,value)]},results:['1','2',String(value)]}
          }
          output={ok:true,workbookManual:{version:3,origin:'manual',changeId,baseRevision:base,saveOperationId:'save_manual_cell',undoOperationId:'undo_manual_cell',workbook:preparedWorkbook,candidate:candidate()}};break
        case 'commit-object':
          if(fault==='commit-unknown')throw Error('lost reply')
          if(fault==='commit-outer')return {ok:false,code:'adapter_unavailable',message:'PRIVATE_COMMIT_MESSAGE_CANARY'}
          if(fault==='commit-invalid-outer')return {ok:false,code:'PRIVATE_OUTER_CODE_CANARY',message:'PRIVATE_COMMIT_MESSAGE_CANARY'} as unknown as PluginPackageHostResponse
          if(fault==='commit-invalid-inner'){output={ok:false,code:'PRIVATE_INNER_CODE_CANARY',message:'PRIVATE_COMMIT_MESSAGE_CANARY'};break}
          if(fault==='commit-rejected'){output={ok:false,code:innerCode,message:'PRIVATE_COMMIT_MESSAGE_CANARY'};break}
          if(fault==='commit-conflict'){output={ok:true,receipt:{operationId:'save_manual_cell',revision:'',status:'conflict',savedAt:''}};break}
          saved=true;persistedBytes=fault==='readback-conflict'?external:candidateBytes
          if(fault==='commit-lost-committed')throw Error('lost committed reply')
          output={ok:true,receipt:{operationId:'save_manual_cell',revision:next,status:'committed',savedAt:'2026-10-19T00:00:00Z'}};break
        case 'object-status':output={ok:true,receipt:{operationId:'save_manual_cell',revision:saved?next:'',status:saved?'committed':'unknown',savedAt:saved?'2026-10-19T00:00:00Z':''}};break
        case 'cancel-change':prepared=false;output={ok:true,recovery:{current:null,pending:null}};break
        case 'close-object':output={ok:true,closed:true};break
        default:throw Error('unexpected operation '+r.operation)
      }
      return {ok:true,output}
    }})
  const open=()=>controller.request({action:'open',workspace:'/workspace',path:'simple.xlsx',threadId:'thread_main',bounds})
  const edit=()=>controller.request({action:'editWorkbookCell',objectId,revision:base,expectedChangeSequence:0,threadId:'thread_main',selectionToken:'selected_cell_token',edit:{type:'formula',formula:'=SUM(A1:A2)'}})
  const differentEdit=()=>controller.request({action:'editWorkbookCell',objectId,revision:base,expectedChangeSequence:0,threadId:'thread_main',selectionToken:'selected_cell_token',edit:{type:'number',value:42}})
  return {controller,calls,hostCalls,open,edit,differentEdit,setPartial:()=>{partial=true},get destroyed(){return destroyed},get created(){return created}}
}
describe('protected local workbook manual controller',()=>{
  it('revalidates the original native target and commits the exact Core candidate before readonly reopen',async()=>{
    const h=harness();expect((await h.open()).ok).toBe(true)
    const result=await h.edit();expect(result.ok).toBe(true);expect(nativeOfficeResponseSchema.safeParse(result).success).toBe(true)
    expect(result.view?.revision).toBe(next);expect(result.view?.recovery?.current).toMatchObject({version:3,origin:'manual',status:'committed',changeId})
    expect(result.view).toMatchObject({status:'ready',dirty:false,editing:false,saving:false,canUndo:true,changeSequence:0,acknowledgedSequence:0})
    expect(h.created).toBe(2);expect(h.destroyed).toBe(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='open-object')).toHaveLength(2)
    expect(h.calls.filter(r=>r.command==='open')).toEqual([expect.objectContaining({version:base,bytes:original}),expect.objectContaining({version:next,bytes:candidateBytes})])
    const ops=h.hostCalls.flatMap(r=>r.action==='invoke'?[r.operation]:[])
    expect(ops).toContain('prepare-workbook-manual');expect(ops.some(op=>op.startsWith('model-')||op.startsWith('proposal-'))).toBe(false)
    const reads=h.calls.filter(r=>r.command==='captureCells')
    expect(reads).toHaveLength(2)
    for(const read of reads)expect(read).toMatchObject({version:base,selectionToken:'selected_cell_token',expectedChangeSequence:0,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}})
    expect(reads[0].operationId).not.toBe(reads[1].operationId)
    expect(h.calls.filter(r=>r.command==='edit')).toHaveLength(1)
    expect(h.calls.some(r=>['replaceCells','export','ack','manual-edit'].includes(r.command))).toBe(false)
    const prepare=h.hostCalls.find(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')
    expect(prepare&&'input'in prepare&&prepare.input.writeTarget).toEqual({sheet:0,sheetName:'Sheet1',column:0,row:2})
    const commit=h.hostCalls.find(r=>r.action==='invoke'&&r.operation==='commit-object')
    expect(commit&&'input'in commit&&commit.input).toEqual({sessionId,threadId:'thread_main',changeId,operationId:'save_manual_cell',baseRevision:base,
      content:{encoding:'base64',kind:'xlsx',byteLength:candidateBytes.byteLength,sha256:next,data:Buffer.from(candidateBytes).toString('base64')}})
    expect(JSON.stringify(result)).not.toContain(Buffer.from(candidateBytes).toString('base64'))
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
    await h.controller.request({action:'saveStatus',objectId})
    expect(logNativeOfficeSaveFailure).toHaveBeenLastCalledWith('unknown',{phase:'query',outerSchemaAccepted:true,innerSchemaAccepted:true,outerCode:null,innerCode:null,receiptPresent:true,receiptStatus:'unknown'})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='object-status')).toHaveLength(2)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')).toHaveLength(1)
    expect(h.calls.filter(r=>r.command==='captureCells')).toHaveLength(2)
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
  })
  it.each(['candidate-hash','candidate-length','candidate-change','candidate-save','candidate-base','candidate-kind'] as const)('rejects %s before committing or touching native mutation state',async fault=>{
    const h=harness(fault);await h.open()
    expect(await h.edit()).toMatchObject({ok:false,error:'invalid_response',view:{revision:base,saving:false}})
    expect(h.hostCalls.some(r=>r.action==='invoke'&&r.operation==='commit-object')).toBe(false)
    expect(h.calls.some(r=>['replaceCells','export','ack','manual-edit'].includes(r.command))).toBe(false)
    expect(h.calls.filter(r=>r.command==='open')).toHaveLength(1)
  })
  it.each(['late-capture-changed','late-selection-changed'] as const)('does not commit after %s during the final native revalidation',async fault=>{
    const h=harness(fault);await h.open()
    expect(await h.edit()).toMatchObject({ok:false,error:'stale_selection',view:{revision:base,saving:false}})
    expect(h.calls.filter(r=>r.command==='captureCells')).toHaveLength(2)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')).toHaveLength(1)
    expect(h.hostCalls.some(r=>r.action==='invoke'&&r.operation==='commit-object')).toBe(false)
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
  })
  it.each(['prepare-lost','candidate-failed'] as const)('recovers the durable %s reservation and allows explicit cancellation without commit',async fault=>{
    const h=harness(fault);await h.open()
    expect((await h.edit()).ok).toBe(false)
    expect(h.controller.getView()).toMatchObject({revision:base,saving:false,recovery:{pending:{version:3,origin:'manual',changeId,status:'prepared',canCancel:true,canResume:false}}})
    const cancelled=await h.controller.request({action:'cancelChange',objectId,revision:base,expectedChangeSequence:0,threadId:'thread_main',changeId})
    expect(cancelled).toMatchObject({ok:true,view:{revision:base,status:'ready'}})
    expect(cancelled.view?.saving).not.toBe(true)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='cancel-change')).toHaveLength(1)
    expect(h.hostCalls.some(r=>r.action==='invoke'&&r.operation==='commit-object')).toBe(false)
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
  })
  it.each(['prepare-lost','candidate-failed'] as const)('replays the same prepare request after %s instead of creating a new authority',async fault=>{
    const h=harness(fault);await h.open();expect((await h.edit()).ok).toBe(false)
    expect((await h.edit()).ok).toBe(true)
    const preparations=h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')
    expect(preparations).toHaveLength(2)
    expect(preparations[1]).toEqual(preparations[0])
    expect(h.calls.filter(r=>r.command==='edit')).toHaveLength(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
  })
  it('does not reuse a lost preparation for a different edit of the same cell',async()=>{
    const h=harness('prepare-lost');await h.open();expect((await h.edit()).ok).toBe(false)
    const changed=await h.controller.request({action:'editWorkbookCell',objectId,revision:base,expectedChangeSequence:0,threadId:'thread_main',
      selectionToken:'selected_cell_token',edit:{type:'number',value:42}})
    expect(changed.ok).toBe(false)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')).toHaveLength(1)
    expect(h.hostCalls.some(r=>r.action==='invoke'&&r.operation==='commit-object')).toBe(false)
  })
  it.each(['prepare-rejected','prepare-rejected-old-current','prepare-rejected-undone-current'] as const)('releases %s only after successful recovery proves no new reservation so discard can close',async fault=>{
    const h=harness(fault);await h.open()
    expect(await h.edit()).toMatchObject({ok:false,error:'conflict',view:{revision:base,saving:false}})
    expect(h.controller.getView()?.recovery?.pending).toBeNull()
    expect(await h.controller.request({action:'close',objectId,discard:true})).toEqual({ok:true,view:null})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')).toHaveLength(1)
    expect(h.hostCalls.some(r=>r.action==='invoke'&&['commit-object','cancel-change'].includes(r.operation))).toBe(false)
  })
  it.each(['prepare-rejected','prepare-rejected-old-current','prepare-rejected-undone-current'] as const)('allows a different new edit after %s with no retained reservation',async fault=>{
    const h=harness(fault);await h.open();expect((await h.edit()).error).toBe('conflict')
    expect(await h.differentEdit()).toMatchObject({ok:true,view:{revision:next,status:'ready',saving:false}})
    const preparations=h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')
    expect(preparations).toHaveLength(2)
    const inputs=preparations.flatMap(r=>r.action==='invoke'?[r.input]:[])
    expect(inputs[1].operationId).not.toBe(inputs[0].operationId)
    expect(inputs[1].edit).toEqual({type:'number',value:42})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
  })
  it.each(['prepare-rejected-query-failed','prepare-lost-empty','prepare-rejected-new-current','candidate-hash-current'] as const)('retains %s when recovery cannot prove this attempt absent',async fault=>{
    const h=harness(fault);await h.open();expect((await h.edit()).ok).toBe(false)
    expect(h.controller.getView()).toMatchObject({revision:base,saving:false})
    expect(await h.controller.request({action:'close',objectId,discard:true})).toMatchObject({ok:false,error:'unknown'})
    expect((await h.differentEdit()).ok).toBe(false)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')).toHaveLength(1)
    expect(h.hostCalls.some(r=>r.action==='invoke'&&['commit-object','cancel-change'].includes(r.operation))).toBe(false)
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
  })
  it.each(['invalid_request','not_text','too_large','persistence_failure'] as const)('retains Core %s diagnostics while preserving save_failed and the original revision',async code=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness('commit-rejected',true,code);await h.open()
    expect(await h.edit()).toMatchObject({ok:false,error:'save_failed',view:{revision:base,dirty:false}})
    expect(logNativeOfficeSaveFailure).toHaveBeenCalledExactlyOnceWith('save_failed',{phase:'commit',outerSchemaAccepted:true,innerSchemaAccepted:true,outerCode:null,innerCode:code,receiptPresent:false,receiptStatus:null})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
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
  it('records a Core conflict without acknowledging an invented native export',async()=>{
    vi.mocked(logNativeOfficeSaveFailure).mockClear()
    const h=harness('commit-conflict');await h.open();expect((await h.edit()).error).toBe('conflict')
    expect(logNativeOfficeSaveFailure).toHaveBeenCalledExactlyOnceWith('conflict',{phase:'commit',outerSchemaAccepted:true,innerSchemaAccepted:true,outerCode:null,innerCode:null,receiptPresent:true,receiptStatus:'conflict'})
    expect(h.calls.some(r=>r.command==='ack')).toBe(false)
  })
  it('resolves a lost committed reply from status and reopens the receipted bytes without preparing or committing again',async()=>{
    const h=harness('commit-lost-committed');await h.open()
    expect(await h.edit()).toMatchObject({ok:false,error:'unknown',view:{revision:base,saving:true,canUndo:false}})
    const result=await h.controller.request({action:'saveStatus',objectId})
    expect(result).toMatchObject({ok:true,view:{revision:next,dirty:false,canUndo:true,status:'ready',editing:false,saving:false,changeSequence:0,acknowledgedSequence:0}})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='object-status')).toHaveLength(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')).toHaveLength(1)
    expect(h.calls.filter(r=>r.command==='open')).toHaveLength(2)
    expect(h.calls.filter(r=>r.command==='open')[1]).toMatchObject({version:next,bytes:candidateBytes})
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
  })
  it('rejects a changed Core readback without loading the external bytes into the native view',async()=>{
    const h=harness('readback-conflict');await h.open();const result=await h.edit()
    expect(result).toMatchObject({ok:false,error:'conflict',view:{saving:false,editing:false,canUndo:false}})
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='open-object')).toHaveLength(2)
    expect(h.calls.filter(r=>r.command==='open')).toEqual([expect.objectContaining({version:base,bytes:original})])
    expect(h.created).toBe(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
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
    expect(h.calls.filter(r=>r.command==='open')).toEqual([expect.objectContaining({version:base,bytes:original}),expect.objectContaining({version:next,bytes:candidateBytes}),expect.objectContaining({version:next,bytes:candidateBytes})])
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='commit-object')).toHaveLength(1)
    expect(h.hostCalls.filter(r=>r.action==='invoke'&&r.operation==='prepare-workbook-manual')).toHaveLength(1)
    expect(h.calls.some(r=>['replaceCells','export','ack'].includes(r.command))).toBe(false)
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
