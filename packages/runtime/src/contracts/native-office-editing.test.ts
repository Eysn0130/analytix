import { createHash } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { nativeWorkbookCandidateSchema, nativeWorkbookManualAuthoritySchema, nativeWorkbookSelectionSchema, nativeWorkbookPatchSchema, MaxNativeOfficeBytes, nativeOfficeCommitInputSchema, nativeOfficeStatusInputSchema, nativeOfficeSelectionCaptureInputSchema, nativeOfficeProposalRejectInputSchema, nativeOfficeProposalDecisionInputSchema, nativeOfficeSelectionResponseSchema, nativeOfficeProposalSchema, nativeOfficeReplacementSchema, nativeOfficeChangeSchema, nativeOfficeLocalReviewSchema, nativeOfficeRecoverySchema } from './native-office-editing'
import { objectEditingRequestSchema, objectEditingResponseSchema } from './object-editing'
import { nativeOfficeCreateInputSchema, nativeOfficeManualBeginInputSchema, nativeOfficeManualAuthoritySchema, nativeOfficeAnnotationSchema } from './native-office-editing'
import { nativeOfficeRequestSchema } from '../../../../src/shared/native-office'
import { pluginPackageViewSchema } from './plugin-package-host'

const input = () => ({ sessionId: 'a'.repeat(48), operationId: 'save_0001', threadId:'thread_main', changeId:'c'.repeat(64), baseRevision: 'b'.repeat(64),
  content: { encoding: 'base64', kind: 'docx', byteLength: 1, sha256: createHash('sha256').update('a').digest('hex'), data: 'YQ==' } })

describe('protected-local annotation drafts', () => {
  const annotation = {objectId:'a'.repeat(64),threadId:'thread_main',draftRevision:'',note:'',sourceRevision:'',updatedAt:''}
  it('restores bounded notes without restoring selection authority', () => {
    expect(nativeOfficeAnnotationSchema.safeParse(annotation).success).toBe(true)
    for (const note of ['中'.repeat(4096),'😀'.repeat(2048)]) {
      expect(nativeOfficeAnnotationSchema.safeParse({...annotation,note}).success).toBe(true)
      expect(nativeOfficeAnnotationSchema.safeParse({...annotation,note:note+'x'}).success).toBe(false)
    }
    expect(nativeOfficeAnnotationSchema.safeParse({...annotation,note:'\ud800'}).success).toBe(false)
    for (const authority of [{selectionToken:'selection_01'},{editable:true},{scopeId:'c'.repeat(48)},{path:'/private'}]) {
      expect(nativeOfficeAnnotationSchema.safeParse({...annotation,...authority}).success).toBe(false)
    }
    const request = {action:'annotationSave',objectId:annotation.objectId,threadId:annotation.threadId,note:'备注',sourceRevision:'b'.repeat(64)}
    expect(nativeOfficeRequestSchema.safeParse(request).success).toBe(true)
    expect(nativeOfficeRequestSchema.safeParse({...request,expectedDraftRevision:'c'.repeat(64)}).success).toBe(false)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ok:true,annotation}).success).toBe(true)
  })
  it('bounds the expanded operation catalog for two finite annotation operations', () => {
    const operations = pluginPackageViewSchema.shape.operations
    expect(operations.safeParse(Array.from({length:20},(_,i)=>'operation-'+i)).success).toBe(true)
    expect(operations.safeParse(Array.from({length:21},(_,i)=>'operation-'+i)).success).toBe(false)
  })
})

describe('protected-local native Office input', () => {
  it('supports three explicit kinds without widening the text contract', () => {
    for (const kind of ['docx', 'xlsx', 'pptx']) {
      const request = input(); request.content.kind = kind
      expect(nativeOfficeCommitInputSchema.safeParse(request).success).toBe(true)
      expect(objectEditingRequestSchema.safeParse({ action: 'commit', ...request }).success).toBe(false)
    }
    expect(nativeOfficeStatusInputSchema.safeParse({ sessionId: input().sessionId, operationId: input().operationId }).success).toBe(true)
    expect(objectEditingResponseSchema.safeParse({ ok: true, receipt: { operationId: 'save_0001', revision: 'c'.repeat(64), status: 'committed', savedAt: '2026-09-14T00:00:00Z' } }).success).toBe(true)
  })
  it('rejects ambiguous encoding, length, digest and authority input', () => {
    for (const delta of [{ data: 'YR==' }, { data: 'YQ==\n' }, { data: 'YQ' }, { data: 'YQ===' }, { data: 'Y===' }, { byteLength: 2 }, { byteLength: 0 }, { byteLength: 1.5 }, { byteLength: MaxNativeOfficeBytes + 1 }, { sha256: 'A'.repeat(64) }, { kind: 'doc' }, { encoding: 'utf-8' }, { path: '/private' }]) {
      const request = input()
      expect(nativeOfficeCommitInputSchema.safeParse({ ...request, content: { ...request.content, ...delta } }).success).toBe(false)
    }
    expect(nativeOfficeCommitInputSchema.safeParse({ ...input(), path: '/private' }).success).toBe(false)
    expect(nativeOfficeStatusInputSchema.safeParse(input()).success).toBe(false)
  })
  it('accepts the exact decoded limit and rejects a byte over', () => {
    for (const size of [MaxNativeOfficeBytes, MaxNativeOfficeBytes + 1]) {
      const request = input()
      const bytes = Buffer.alloc(size, 97)
      request.content = { ...request.content, data: bytes.toString('base64'), byteLength: size }
      expect(nativeOfficeCommitInputSchema.safeParse(request).success).toBe(size === MaxNativeOfficeBytes)
    }
  })
})


describe('native selection proposal transport', () => {
  const capture = () => ({ sessionId: 'a'.repeat(48), threadId: 'thread_main', selectionToken: 'selection_01', changeSequence: 3, baseRevision: 'b'.repeat(64), text: 'Selected text', editable: true })
  it('keeps captures, decision targets and approved replacement on explicit private shapes', () => {
    expect(nativeOfficeSelectionCaptureInputSchema.safeParse(capture()).success).toBe(true)
    expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), text: '\ud800' }).success).toBe(false)
    expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), command: '.uno:Shell' }).success).toBe(false)
    const reject = { sessionId: 'a'.repeat(48), scopeId: 'c'.repeat(48), proposalId: 'd'.repeat(48), operationId: 'reject_001' }
    expect(nativeOfficeProposalRejectInputSchema.safeParse(reject).success).toBe(true)
    expect(nativeOfficeProposalDecisionInputSchema.safeParse(reject).success).toBe(false)
    expect(nativeOfficeProposalDecisionInputSchema.safeParse({ ...reject, selectionToken: 'selection_01', changeSequence: 3, baseRevision: 'b'.repeat(64) }).success).toBe(true)
    const scope = { ...capture(), scopeId: 'c'.repeat(48), parts: [{ kind: 'literal', text: 'Projected text' }] }
    const { text: _privateText, ...projectedScope } = scope
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, scope: projectedScope }).success).toBe(true)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, scope }).success).toBe(false)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, proposals: [{ proposalId: reject.proposalId, status: 'approved', parts: scope.parts }], localReviews:[] }).success).toBe(true)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, proposals: [{ proposalId: reject.proposalId, status: 'applied', parts: scope.parts }], localReviews:[] }).success).toBe(false)
  })
  it('uses the native 4096 UTF-16 bound without reducing discussion or generic text editing', () => {
    for (const text of ['x'.repeat(4096), 'é'.repeat(4096), '😀'.repeat(2048)]) {
      const proposal = { proposalId: 'c'.repeat(48), status: 'proposed', parts: [{ kind: 'literal', text }] }
      const replacement = { proposalId: 'c'.repeat(48), operationId: 'accept_001', changeId:'e'.repeat(64), saveOperationId:'save_core_0001', text, selectionToken: 'selection_01', changeSequence: 3, baseRevision: 'b'.repeat(64) }
      expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), text }).success).toBe(true)
      expect(nativeOfficeProposalSchema.safeParse(proposal).success).toBe(true)
      expect(nativeOfficeReplacementSchema.safeParse(replacement).success).toBe(true)
      expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), text: text + 'x' }).success).toBe(false)
      expect(nativeOfficeProposalSchema.safeParse({ ...proposal, parts: [...proposal.parts, { kind: 'literal', text: 'x' }] }).success).toBe(false)
      expect(nativeOfficeReplacementSchema.safeParse({ ...replacement, text: text + 'x' }).success).toBe(false)
      expect(nativeOfficeSelectionCaptureInputSchema.safeParse({ ...capture(), editable: false, text: text + 'x' }).success).toBe(true)
      expect(objectEditingRequestSchema.safeParse({ action: 'commit', sessionId: capture().sessionId, baseRevision: capture().baseRevision, operationId: 'generic_01', content: text + 'x' }).success).toBe(true)
    }
  })
  it('strictly accepts the shared Core revocation acknowledgement', () => {
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, revoked: true }).success).toBe(true)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, revoked: false }).success).toBe(false)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, revoked: true, text: 'unexpected' }).success).toBe(false)
  })
})


describe('durable protected-local recovery contracts',()=>{
  const change=()=>({changeId:'e'.repeat(64),threadId:'thread_main',proposalId:'c'.repeat(48),baseRevision:'b'.repeat(64),revision:'d'.repeat(64),status:'committed',beforeText:'原始字段',afterText:'修改后字段',saveOperationId:'save_core_0001',undoOperationId:'undo_core_0001',canUndo:true,canCancel:false,canRetryUndo:false,canResume:false,createdAt:'2026-09-15T00:00:00Z',savedAt:'2026-09-15T00:01:00Z'})
  it('requires approval identity on commits and replacements',()=>{
    for(const field of ['changeId','threadId']){
      const request:Record<string,unknown>=input();delete request[field]
      expect(nativeOfficeCommitInputSchema.safeParse(request).success).toBe(false)
    }
    const replacement={proposalId:'c'.repeat(48),operationId:'accept_001',changeId:'e'.repeat(64),saveOperationId:'save_core_0001',text:'Approved',selectionToken:'selection_01',changeSequence:0,baseRevision:'b'.repeat(64)}
    expect(nativeOfficeReplacementSchema.safeParse(replacement).success).toBe(true)
    for(const field of ['changeId','saveOperationId']){
      const incomplete:Record<string,unknown>={...replacement};delete incomplete[field]
      expect(nativeOfficeReplacementSchema.safeParse(incomplete).success).toBe(false)
    }
  })
  it('separates protected raw reviews from projected proposal parts',()=>{
    const proposal={proposalId:'c'.repeat(48),status:'proposed',parts:[{kind:'literal',text:'Projected'}]}
    const review={proposalId:proposal.proposalId,beforeText:'原始字段',afterText:'修改后字段'}
    expect(nativeOfficeSelectionResponseSchema.safeParse({ok:true,proposals:[proposal],localReviews:[review]}).success).toBe(true)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ok:true,proposals:[proposal]}).success).toBe(false)
    expect(nativeOfficeProposalSchema.safeParse({...proposal,beforeText:review.beforeText}).success).toBe(false)
    expect(nativeOfficeLocalReviewSchema.safeParse({...review,threadId:'other'}).success).toBe(false)
    expect(nativeOfficeLocalReviewSchema.safeParse({...review,afterText:'\ud800'}).success).toBe(false)
  })
  it('accepts bounded lifecycle records and rejects omitted capabilities or unknown authority fields',()=>{
    expect(nativeOfficeRecoverySchema.safeParse({current:change(),pending:null}).success).toBe(true)
    expect(nativeOfficeRecoverySchema.safeParse({current:null,pending:{...change(),status:'unknown',canUndo:false,canResume:true}}).success).toBe(true)
    expect(nativeOfficeChangeSchema.safeParse({...change(),canResume:'true'}).success).toBe(false)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ok:true,recovery:{current:change(),pending:null}}).success).toBe(true)
    for(const field of ['threadId','saveOperationId','undoOperationId','canUndo','canCancel','canRetryUndo','canResume']){
      const incomplete:Record<string,unknown>=change();delete incomplete[field]
      expect(nativeOfficeChangeSchema.safeParse(incomplete).success).toBe(false)
    }
    expect(nativeOfficeChangeSchema.safeParse({...change(),path:'/private'}).success).toBe(false)
    expect(nativeOfficeChangeSchema.safeParse({...change(),beforeText:'x'.repeat(65537)}).success).toBe(false)
    expect(nativeOfficeRecoverySchema.safeParse({current:change(),pending:null,raw:'unexpected'}).success).toBe(false)
  })
})


describe('finite typed workbook transport',()=>{
 const cell={sheet:0,column:0,row:0,text:'1',formula:'1',value:1,valueType:'number',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}
 const workbook={sheet:0,sheetName:'统计',startColumn:0,endColumn:0,startRow:0,endRow:255,cells:Array.from({length:256},(_,row)=>({...cell,row}))}
 it('requires a complete ordered unhidden rectangle and explicit value types',()=>{
  expect(nativeWorkbookSelectionSchema.safeParse(workbook).success).toBe(true)
  for(const bad of [{...workbook,cells:workbook.cells.slice(1)},{...workbook,cells:[...workbook.cells,{...cell,row:256}],endRow:256},{...workbook,cells:workbook.cells.map((c,i)=>i?c:{...c,merged:true})},{...workbook,cells:workbook.cells.map((c,i)=>i?c:{...c,columnVisible:false})}])expect(nativeWorkbookSelectionSchema.safeParse(bad).success).toBe(false)
  for(const patch of [{kind:'number',value:2},{kind:'formula',formula:'SUM(A1:A2)'},{kind:'range',cells:[{rowOffset:0,columnOffset:0,type:'number',value:2}]}])expect(nativeWorkbookPatchSchema.safeParse(patch).success).toBe(true)
  for(const patch of [{kind:'number',value:'2'},{kind:'formula',formula:'1',value:1},{kind:'range',cells:[{rowOffset:0,columnOffset:0,type:'number',value:2,path:'/private'}]},{kind:'range',cells:[]}])expect(nativeWorkbookPatchSchema.safeParse(patch).success).toBe(false)
 })
})

describe('Core workbook candidates on existing protected-local responses',()=>{
 const cell={sheet:0,column:0,row:0,text:'2',formula:'2',value:2,valueType:'number',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}
 const before={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0,cells:[cell]}
 const workbook={before,after:{...before,cells:[{...cell,text:'42',formula:'42',value:42}]},results:['42']}
 const candidate={changeId:'c'.repeat(64),saveOperationId:'native_save_0001',baseRevision:'b'.repeat(64),content:{...input().content,kind:'xlsx'}}
 const manual={version:3,origin:'manual',changeId:candidate.changeId,saveOperationId:candidate.saveOperationId,baseRevision:candidate.baseRevision,undoOperationId:'native_undo_0001',workbook,candidate}
 const replacement={proposalId:'d'.repeat(48),operationId:'accept_0001',changeId:candidate.changeId,saveOperationId:candidate.saveOperationId,baseRevision:candidate.baseRevision,text:'42',selectionToken:'selection_01',changeSequence:0,workbook,candidate}
 it('requires candidate bytes and approval identity together without a new operation',()=>{
  expect(nativeWorkbookCandidateSchema.safeParse(candidate).success).toBe(true)
  expect(nativeOfficeSelectionResponseSchema.safeParse({ok:true,workbookManual:manual}).success).toBe(true)
  expect(nativeOfficeSelectionResponseSchema.safeParse({ok:true,replacement}).success).toBe(true)
  for(const delta of [{changeId:'e'.repeat(64)},{baseRevision:'e'.repeat(64)},{saveOperationId:'different_save_01'}]) {
   expect(nativeWorkbookManualAuthoritySchema.safeParse({...manual,candidate:{...candidate,...delta}}).success).toBe(false)
   expect(nativeOfficeReplacementSchema.safeParse({...replacement,candidate:{...candidate,...delta}}).success).toBe(false)
  }
  for(const bad of [undefined,{...candidate,content:{...candidate.content,kind:'docx'}},{...candidate,content:{...candidate.content,data:'YR=='}},{...candidate,path:'/private'}]) {
   expect(nativeWorkbookManualAuthoritySchema.safeParse({...manual,candidate:bad}).success).toBe(false)
   expect(nativeOfficeReplacementSchema.safeParse({...replacement,candidate:bad}).success).toBe(false)
  }
 })
 it('keeps candidates out of projected proposals, reviews and durable recovery records',()=>{
  const proposal={proposalId:replacement.proposalId,status:'approved',parts:[],workbook:{kind:'number',value:42}}
  expect(nativeOfficeProposalSchema.safeParse({...proposal,candidate}).success).toBe(false)
  expect(nativeOfficeLocalReviewSchema.safeParse({proposalId:replacement.proposalId,beforeText:'2',afterText:'42',workbook,candidate}).success).toBe(false)
  expect(nativeOfficeReplacementSchema.safeParse({...replacement,workbook:undefined}).success).toBe(false)
  const record={version:3,origin:'manual',changeId:candidate.changeId,threadId:'thread_main',baseRevision:candidate.baseRevision,revision:'',status:'prepared',saveOperationId:candidate.saveOperationId,undoOperationId:'native_undo_0001',workbook,canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''}
  expect(nativeOfficeChangeSchema.safeParse(record).success).toBe(true)
  expect(nativeOfficeChangeSchema.safeParse({...record,candidate}).success).toBe(false)
 })
})


describe('explicit user Word authority', () => {
  const begin = { sessionId: 'a'.repeat(48), threadId: 'thread_main', operationId: 'manual_begin_001', baseRevision: 'b'.repeat(64) }
  const authority = { version: 2, origin: 'manual', changeId: 'c'.repeat(64), baseRevision: begin.baseRevision, saveOperationId: 'native_save_001', undoOperationId: 'native_undo_001' }
  it('admits a basename and current thread without caller workspace authority', () => {
    const request = { threadId: begin.threadId, operationId: 'create_docx_001', fileName: '调研.docx' }
    expect(nativeOfficeCreateInputSchema.safeParse(request).success).toBe(true)
    for (const fileName of ['../调研.docx', '/tmp/new.docx', 'new.md', '.docx', ' bad.docx', 'bad\\name.docx', 'bad\u0000.docx', '中'.repeat(84) + '.docx']) {
      expect(nativeOfficeCreateInputSchema.safeParse({ ...request, fileName }).success).toBe(false)
    }
    expect(nativeOfficeCreateInputSchema.safeParse({ ...request, workspace: '/private' }).success).toBe(false)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, creation: { operationId: request.operationId, workspace: '/workspace', path: '/workspace/调研.docx', revision: 'd'.repeat(64), kind: 'docx', byteSize: 1024, savedAt: '2026-10-15T00:00:00Z' } }).success).toBe(true)
  })
  it('requires a tagged manual authority and retains strict legacy records', () => {
    expect(nativeOfficeManualBeginInputSchema.safeParse(begin).success).toBe(true)
    expect(nativeOfficeManualBeginInputSchema.safeParse({ ...begin, proposalId: 'p' }).success).toBe(false)
    expect(nativeOfficeManualAuthoritySchema.safeParse(authority).success).toBe(true)
    expect(nativeOfficeSelectionResponseSchema.safeParse({ ok: true, manual: authority }).success).toBe(true)
    const manual = { ...authority, threadId: begin.threadId, revision: '', status: 'prepared', canUndo: false, canCancel: true, canRetryUndo: false, canResume: false, createdAt: '2026-10-15T00:00:00Z', savedAt: '' }
    expect(nativeOfficeChangeSchema.safeParse(manual).success).toBe(true)
    for (const extra of [{ proposalId: 'a'.repeat(48) }, { beforeText: '' }, { origin: 'proposal' }, { version: 1 }]) {
      expect(nativeOfficeChangeSchema.safeParse({ ...manual, ...extra }).success).toBe(false)
    }
    const { version: _version, origin: _origin, ...untagged } = manual
    expect(nativeOfficeChangeSchema.safeParse(untagged).success).toBe(false)
  })
})

describe('protected local typed workbook manual contract',()=>{
  const request={action:'editWorkbookCell',objectId:'a'.repeat(64),revision:'b'.repeat(64),expectedChangeSequence:0,selectionToken:'cell_selection',threadId:'thread_main'}
  it('accepts explicit finite types without broad native/path or proposal authority',()=>{
    for(const edit of [{type:'number',value:12},{type:'text',text:'0012'},{type:'formula',formula:'=SUM(A1:A2)'},{type:'empty'}]) {
      expect(nativeOfficeRequestSchema.safeParse({...request,edit}).success).toBe(true)
      expect(nativeOfficeRequestSchema.safeParse({...request,edit,path:'/private/file.xlsx'}).success).toBe(false)
      expect(nativeOfficeRequestSchema.safeParse({...request,edit,proposalId:'d'.repeat(48)}).success).toBe(false)
    }
    for(const edit of [{type:'number',value:'12'},{type:'number',value:NaN},{type:'formula',formula:''},{type:'empty',text:''},{type:'text',text:'a',formula:'=1'}])
      expect(nativeOfficeRequestSchema.safeParse({...request,edit}).success).toBe(false)
  })
  it('keeps v2 manual shape closed against typed workbook evidence',()=>{
    const manual={version:2,origin:'manual',changeId:'a'.repeat(64),threadId:'thread_main',baseRevision:'b'.repeat(64),revision:'',status:'unknown',saveOperationId:'save_manual_01',undoOperationId:'undo_manual_01',canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''}
    expect(nativeOfficeChangeSchema.safeParse(manual).success).toBe(true)
    expect(nativeOfficeChangeSchema.safeParse({...manual,workbook:{}}).success).toBe(false)
    expect(nativeOfficeChangeSchema.safeParse({...manual,version:3}).success).toBe(true)
    expect(nativeOfficeChangeSchema.safeParse({...manual,version:3,status:'committed'}).success).toBe(false)
  })
})
