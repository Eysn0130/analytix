import { describe, expect, it } from 'vitest'
import { nativeOfficeActionMenuSchema, nativeOfficeActionChoiceSchema, nativeOfficeFormatChoiceSchema, nativeOfficeFormattingSchema, nativeOfficeMenuTargetSchema, nativeOfficePlaybackSchema, nativeOfficeFormatSchema, nativeOfficeFormatStateSchema, nativeOfficeRangeLeaseSchema, nativeOfficeFreezeAckSchema, nativeOfficeRequestSchema, nativeOfficeResponseSchema, nativeOfficeViewSchema } from './native-office'
import { isOfficeFormat, isOfficeFormatState, isOfficeFormatting, isOfficeRangeLease, isOfficeFreezeAck, isOfficePointerMessage, isOfficePlaybackState, isOfficePreviewView, isOfficeRequest, isOfficeResult } from '../main/office/office-protocol'

const target = { objectId: 'a'.repeat(64), revision: 'b'.repeat(64), expectedChangeSequence: 0 }
const wire = { command: 'local-view', operationId: 'operation', documentId: target.objectId, version: target.revision, channel: 'channel' }
const idle = { generation: 0, sequence: 0, phase: 'idle', slideIndex: null, slideCount: null }
const running = { generation: 2, sequence: 5, phase: 'running', slideIndex: 0, slideCount: 3 }
const preview = { zoom: 100, zoomType: 0, fit: false, page: 1, pages: 3, readOnly: true, modified: false }
const view = { objectId: target.objectId, revision: target.revision, path: '/synthetic/deck.pptx', kind: 'pptx', dirty: false, status: 'ready' }

it('preserves forbidden Office responses and view errors while rejecting unknown codes or raw details', () => {
  const denied = { ok: false, view: null, error: 'forbidden' }
  const deniedView = { ...view, status: 'error', error: 'forbidden' }
  expect(nativeOfficeResponseSchema.parse(denied)).toEqual(denied)
  expect(nativeOfficeViewSchema.safeParse(deniedView).success).toBe(true)
  expect(nativeOfficeResponseSchema.safeParse({ ...denied, view: deniedView }).success).toBe(true)
  for (const error of ['unknown_denial', 'FORBIDDEN', 'forbidden: /private/office', { code: 'forbidden' }]) {
    expect(nativeOfficeResponseSchema.safeParse({ ...denied, error }).success).toBe(false)
    expect(nativeOfficeViewSchema.safeParse({ ...deniedView, error }).success).toBe(false)
  }
  expect(nativeOfficeResponseSchema.safeParse({ ...denied, message: '/private/office/body-sentinel' }).success).toBe(false)
})

describe('bounded native presentation playback contract', () => {
  it('accepts only bounded sampled playback state, independently of editor page and zoom', () => {
    for (const playback of [idle, running, { ...running, phase: 'paused', slideIndex: -1 }, { ...running, slideIndex: 511, slideCount: 512 },
      ...['unsupported', 'error'].map(phase => ({ ...idle, phase }))]) {
      expect(nativeOfficePlaybackSchema.safeParse(playback).success).toBe(true)
      expect(isOfficePlaybackState(playback)).toBe(true)
    }
    for (const playback of [
      { ...idle, generation: -1 }, { ...idle, sequence: 0.5 }, { ...idle, sequence: Number.MAX_SAFE_INTEGER + 1 },
      { ...running, slideIndex: null }, { ...running, slideCount: null }, { ...running, slideIndex: -2 },
      { ...running, slideIndex: 3 }, { ...running, slideCount: 0 }, { ...running, slideCount: 513 },
      { ...idle, slideIndex: 0, slideCount: 3 }, { ...idle, phase: 'playing' }, { ...idle, phase: ['idle'] }, { ...idle, url: 'file:///private' }
    ]) {
      expect(nativeOfficePlaybackSchema.safeParse(playback).success).toBe(false)
      expect(isOfficePlaybackState(playback)).toBe(false)
    }
    expect(nativeOfficeViewSchema.safeParse(view).success).toBe(true)
    expect(nativeOfficeViewSchema.safeParse({ ...view, playback: running }).success).toBe(true)
    expect(nativeOfficeViewSchema.safeParse({ ...view, playback: running, previewView: { zoom: 25, fit: false, page: 3, pages: 3 } }).success).toBe(true)
    expect(nativeOfficeViewSchema.safeParse({ ...view, kind: 'docx', playback: idle }).success).toBe(false)
    expect(isOfficePreviewView({ ...preview, playback: running }, 'pptx')).toBe(true)
    expect(isOfficePreviewView({ ...preview, page: null, pages: null, playback: idle }, 'docx')).toBe(false)
  })

  it('requires a playback generation exactly for every playback mutation and never for reads or zoom', () => {
    const controls = ['play-start','play-current','play-next-effect','play-previous-effect','play-next-slide','play-previous-slide','play-pause','play-resume','play-end']
    for (const control of controls) {
      expect(nativeOfficeRequestSchema.safeParse({ action: 'viewControl', ...target, control, playbackGeneration: 2 }).success).toBe(true)
      expect(isOfficeRequest({ ...wire, action: control, playbackGeneration: 2 })).toBe(true)
      for (const extra of [{}, { playbackGeneration: undefined }, { playbackGeneration: -1 }, { playbackGeneration: 0.5 },
        { playbackGeneration: 2, zoom: 100 }, { playbackGeneration: 2, uno: '.uno:Presentation' }]) {
        expect(nativeOfficeRequestSchema.safeParse({ action: 'viewControl', ...target, control, ...extra }).success).toBe(false)
        expect(isOfficeRequest({ ...wire, action: control, ...extra })).toBe(false)
      }
    }
    expect(nativeOfficeRequestSchema.safeParse({ action: 'viewControl', ...target, control: 'play-read' }).success).toBe(true)
    expect(isOfficeRequest({ ...wire, action: 'play-read' })).toBe(true)
    for (const control of ['read','fit','play-read']) {
      expect(nativeOfficeRequestSchema.safeParse({ action: 'viewControl', ...target, control, playbackGeneration: 0 }).success).toBe(false)
      expect(isOfficeRequest({ ...wire, action: control, playbackGeneration: 0 })).toBe(false)
    }
    expect(nativeOfficeRequestSchema.safeParse({ action: 'viewControl', ...target, control: 'set-zoom', zoom: 100 }).success).toBe(true)
    expect(nativeOfficeRequestSchema.safeParse({ action: 'viewControl', ...target, control: 'play-read', zoom: undefined }).success).toBe(false)
  })

  it('permits protected PPT playback without admitting edits or transfers', () => {
    const protectedPresent = { threadId: 'thread', turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64),
      resultItemId: 'item_result_' + 'b'.repeat(64), viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64), index: 0 }
    expect(nativeOfficeViewSchema.safeParse({ ...view, protectedPresent, playback: running }).success).toBe(true)
    for (const extra of [{ dirty: true }, { editing: true }, { saving: true }, { changeSequence: 1 }]) {
      expect(nativeOfficeViewSchema.safeParse({ ...view, protectedPresent, playback: running, ...extra }).success).toBe(false)
    }
  })

  it('recognizes bounded presentation failures and rejects playback on non-presentation results', () => {
    for (const error of ['presentation-unsupported','presentation-failed','presentation-start-timeout']) {
      expect(isOfficeResult({ ...wire, type: 'result', ok: false, error })).toBe(true)
    }
    const state = { documentId: target.objectId, version: target.revision, kind: 'docx', changeSequence: 0, acknowledgedSequence: 0, dirty: false }
    expect(isOfficeResult({ ...wire, type: 'result', ok: true, state, view: { ...preview, page: null, pages: null, playback: idle } })).toBe(false)
  })
})

it('bounds pointer presentation to a private selection lease with no geometry, paths or public edit command',()=>{
  const pointer={interactionId:'interaction_id',selectionToken:'selection_token'}
  const menu={...target,pointer,actions:[{id:'quote',label:'Quote',enabled:true}]}
  expect(nativeOfficeMenuTargetSchema.safeParse({...target,pointer}).success).toBe(true)
  expect(nativeOfficeActionMenuSchema.safeParse(menu).success).toBe(true)
  for(const invalid of [{...pointer,x:10},{...pointer,selectionToken:''},{...pointer,path:'/private'}])expect(nativeOfficeActionMenuSchema.safeParse({...menu,pointer:invalid}).success).toBe(false)
  const lease={type:'pointer-actions',phase:'request',...wire,...pointer,expectedChangeSequence:0}
  delete (lease as any).command
  expect(isOfficePointerMessage(lease,'guest')).toBe(true)
  expect(isOfficePointerMessage(lease,'host')).toBe(false)
  expect(isOfficePointerMessage({...lease,x:5},'guest')).toBe(false)
  expect(isOfficePointerMessage({...lease,phase:'present',actions:menu.actions},'host')).toBe(true)
  expect(isOfficePointerMessage({...lease,phase:'present',actions:[...menu.actions,...menu.actions]},'host')).toBe(false)
  expect(nativeOfficeRequestSchema.safeParse({action:'validateSelection',...target,...pointer}).success).toBe(false)
  const request={...wire,command:'validateSelection',selectionToken:pointer.selectionToken,expectedChangeSequence:0}
  expect(isOfficeRequest(request)).toBe(true)
  expect(isOfficeRequest({...request,x:3})).toBe(false)
  const state={documentId:wire.documentId,version:wire.version,kind:'docx',changeSequence:0,acknowledgedSequence:0,dirty:false}
  expect(isOfficeResult({...wire,command:'validateSelection',type:'result',ok:true,state})).toBe(true)
  expect(isOfficeResult({...wire,command:'validateSelection',type:'result',ok:true,state,selection:{}})).toBe(false)
})


describe('explicit manual Word authority and fixed native formatting wire', () => {
  const manual = { version: 2, origin: 'manual', changeId: 'c'.repeat(64), baseRevision: target.revision,
    saveOperationId: 'save_operation', undoOperationId: 'undo_operation' }
  const rangeLease = { generation: 3, sequence: 0, token: 'selection_token' }
  const formatState = { fontFamily: 'Noto Sans CJK SC', fontSize: 11, bold: false, italic: null, underline: true }
  const state = { documentId: target.objectId, version: target.revision, kind: 'docx', changeSequence: 0, acknowledgedSequence: 0, dirty: false, manual: { version: 2, origin: 'manual', changeId: manual.changeId, baseRevision: target.revision, generation: 3, frozen: false } }
  const selection = { documentId: target.objectId, version: target.revision, changeSequence: 0, kind: 'text',
    scope: 'session-text-range-at-version-and-change-sequence', token: rangeLease.token, text: '中文 fixture', rangeLease, formatState }
  const labels = { fontFamily: '字体', fontSize: '字号', bold: '粗体', italic: '斜体', underline: '下划线', mixed: '混合' }
  const formatting = { changeId: manual.changeId, rangeLease, formatState, labels }
  const pointer = { interactionId: 'interaction_id', selectionToken: rangeLease.token }
  const actions = [{ id: 'quote', label: 'Quote', enabled: true }]
  const pointerLease = { type: 'pointer-actions', channel: wire.channel, operationId: wire.operationId,
    documentId: wire.documentId, version: wire.version, expectedChangeSequence: 0, ...pointer }

  it('binds optional formatting menus to the existing pointer lease and bounds all six labels', () => {
    const menu = { ...target, pointer, actions, formatting }
    expect(nativeOfficeActionMenuSchema.safeParse({ ...target, actions }).success).toBe(true)
    expect(nativeOfficeActionMenuSchema.safeParse(menu).success).toBe(true)
    expect(nativeOfficeFormattingSchema.safeParse(formatting).success).toBe(true)
    expect(isOfficeFormatting(formatting)).toBe(true)
    const mixed = { fontFamily: null, fontSize: null, bold: null, italic: null, underline: null }
    expect(nativeOfficeFormattingSchema.safeParse({ ...formatting, formatState: mixed }).success).toBe(true)
    expect(isOfficeFormatting({ ...formatting, formatState: mixed })).toBe(true)
    for (const next of [{ ...target, actions, formatting }, { ...menu, pointer: undefined }, { ...menu, formatting: undefined },
      { ...menu, formatting: { ...formatting, rangeLease: { ...rangeLease, token: 'other_selection' } } },
      { ...menu, expectedChangeSequence: 1 }, { ...menu, formatting: { ...formatting, uno: '.uno:Save' } }]) {
      expect(nativeOfficeActionMenuSchema.safeParse(next).success).toBe(false)
    }
    for (const key of Object.keys(labels)) {
      for (const value of ['', 'a'.repeat(121), 'bad\nlabel', undefined]) {
        const next = { ...formatting, labels: { ...labels, [key]: value } }
        expect(nativeOfficeFormattingSchema.safeParse(next).success).toBe(false)
        expect(isOfficeFormatting(next)).toBe(false)
      }
      const missing: Record<string, string> = { ...labels }; delete missing[key]
      expect(nativeOfficeFormattingSchema.safeParse({ ...formatting, labels: missing }).success).toBe(false)
      expect(isOfficeFormatting({ ...formatting, labels: missing })).toBe(false)
      for (const value of ['字', '字'.repeat(120)]) {
        const next = { ...formatting, labels: { ...labels, [key]: value } }
        expect(nativeOfficeFormattingSchema.safeParse(next).success).toBe(true)
        expect(isOfficeFormatting(next)).toBe(true)
      }
    }
    for (const next of [{ ...formatting, labels: { ...labels, path: '/private' } },
      { ...formatting, changeId: 'change' }, { ...formatting, rangeLease: { ...rangeLease, generation: -1 } },
      { ...formatting, formatState: { ...formatState, fontSize: 11.25 } },
      { ...formatting, formatState: { ...formatState, fontFamily: undefined } }]) {
      expect(nativeOfficeFormattingSchema.safeParse(next).success).toBe(false)
      expect(isOfficeFormatting(next)).toBe(false)
    }
  })

  it('keeps legacy action choices and admits only complete closed format choices', () => {
    for (const actionId of ['quote', null]) expect(nativeOfficeActionChoiceSchema.safeParse({ actionId }).success).toBe(true)
    const formats = [{ kind: 'font-family', value: 'Noto Sans CJK SC' }, { kind: 'font-size', value: 10.5 },
      { kind: 'bold', value: true }, { kind: 'italic', value: false }, { kind: 'underline', value: true }]
    for (const format of formats) {
      const choice = { actionId: null, changeId: manual.changeId, rangeLease, format }
      expect(nativeOfficeFormatChoiceSchema.safeParse(choice).success).toBe(true)
      expect(nativeOfficeActionChoiceSchema.safeParse(choice).success).toBe(true)
    }
    const choice = { actionId: null, changeId: manual.changeId, rangeLease, format: formats[0] }
    for (const next of [{ ...choice, actionId: 'quote' }, { actionId: null, changeId: manual.changeId },
      { ...choice, rangeLease: undefined }, { ...choice, format: undefined },
      { ...choice, format: { kind: 'uno', value: '.uno:Save' } },
      { ...choice, format: { kind: 'bold', value: true, property: 'CharWeight' } },
      { ...choice, dispatch: '.uno:Save' }, { actionId: null, formatting }]) {
      expect(nativeOfficeActionChoiceSchema.safeParse(next).success).toBe(false)
      expect(nativeOfficeFormatChoiceSchema.safeParse(next).success).toBe(false)
    }
  })

  it('validates format pointer messages in one direction with the exact original lease', () => {
    const present = { ...pointerLease, phase: 'present', actions, formatting }
    const choice = { ...pointerLease, phase: 'format', changeId: manual.changeId, rangeLease, format: { kind: 'bold', value: true } }
    expect(isOfficePointerMessage(present, 'host')).toBe(true)
    expect(isOfficePointerMessage(present, 'guest')).toBe(false)
    expect(isOfficePointerMessage(choice, 'guest')).toBe(true)
    expect(isOfficePointerMessage(choice, 'host')).toBe(false)
    for (const next of [{ ...choice, rangeLease: { ...rangeLease, token: 'other_selection' } },
      { ...choice, rangeLease: { ...rangeLease, sequence: 1 } }, { ...choice, actionId: null },
      { ...choice, changeId: 'change' }, { ...choice, format: { kind: 'uno', value: '.uno:Save' } },
      { ...choice, format: { kind: 'italic', value: true, property: 'CharPostureAsian' } },
      { ...pointerLease, phase: 'request', formatting }, { ...pointerLease, phase: 'cancel', formatting },
      { ...pointerLease, phase: 'choice', actionId: 'quote', format: choice.format }]) {
      expect(isOfficePointerMessage(next, 'guest')).toBe(false)
    }
    for (const next of [{ ...present, formatting: undefined },
      { ...present, formatting: { ...formatting, rangeLease: { ...rangeLease, token: 'other_selection' } } },
      { ...present, expectedChangeSequence: 1 }, { ...present, formatting: { ...formatting, labels: { ...labels, mixed: '' } } },
      { ...pointerLease, phase: 'dismiss', formatting }]) {
      expect(isOfficePointerMessage(next, 'host')).toBe(false)
    }
  })

  it('requires explicit version/origin and rejects caller paths on user creation', () => {
    expect(nativeOfficeRequestSchema.safeParse({ action: 'create', threadId: 'thread', operationId: 'create_operation', fileName: '记录.docx', bounds: { x: 0, y: 0, width: 500, height: 500 } }).success).toBe(true)
    expect(nativeOfficeRequestSchema.safeParse({ action: 'create', threadId: 'thread', operationId: 'create_operation', fileName: '../记录.docx', bounds: { x: 0, y: 0, width: 500, height: 500 } }).success).toBe(false)
    expect(nativeOfficeRequestSchema.safeParse({ action: 'beginManualEdit', ...target, threadId: 'thread' }).success).toBe(true)
    expect(isOfficeRequest({ ...wire, command: 'manual-edit', manual })).toBe(true)
    for (const authority of [{ ...manual, origin: undefined }, { ...manual, version: 1 }, { ...manual, baseRevision: 'd'.repeat(64) }, { changeId: manual.changeId }]) {
      expect(isOfficeRequest({ ...wire, command: 'manual-edit', manual: authority })).toBe(false)
    }
    expect(isOfficeRequest({ ...wire, command: 'edit', manual })).toBe(false)
    expect(nativeOfficeViewSchema.safeParse({ ...view, kind: 'docx', editing: true, manual: { version: 2, origin: 'manual', changeId: manual.changeId, baseRevision: target.revision, generation: 3, frozen: false } }).success).toBe(true)
    expect(nativeOfficeViewSchema.safeParse({ ...view, editing: true, manual: { version: 2, origin: 'manual', changeId: manual.changeId, baseRevision: target.revision, generation: 3, frozen: false } }).success).toBe(false)
  })

  it('accepts only the closed property union and a complete native format sample', () => {
    const formats = [{ kind: 'font-family', value: 'Noto Sans CJK SC' }, { kind: 'font-size', value: 10.5 }, { kind: 'bold', value: true }, { kind: 'italic', value: false }, { kind: 'underline', value: true }]
    for (const format of formats) {
      expect(nativeOfficeFormatSchema.safeParse(format).success).toBe(true)
      expect(isOfficeFormat(format)).toBe(true)
      expect(nativeOfficeRequestSchema.safeParse({ action: 'format', ...target, changeId: manual.changeId, rangeLease, format }).success).toBe(true)
      expect(isOfficeRequest({ ...wire, command: 'format', changeId: manual.changeId, rangeLease, format })).toBe(true)
    }
    for (const format of [{ kind: 'uno', value: '.uno:Save' }, { kind: 'font-size', value: 0 }, { kind: 'font-size', value: 11.25 }, { kind: 'font-family', value: 'A\nB' }, { kind: 'bold', value: 1 }, { kind: new String('bold'), value: true }, { kind: 'italic', value: true, property: 'CharPostureAsian' }]) {
      expect(nativeOfficeFormatSchema.safeParse(format).success).toBe(false)
      expect(isOfficeFormat(format)).toBe(false)
    }
    expect(nativeOfficeFormatStateSchema.safeParse(formatState).success).toBe(true)
    expect(isOfficeFormatState(formatState)).toBe(true)
    expect(isOfficeFormatState({ ...formatState, fontSize: undefined })).toBe(false)
    expect(isOfficeRequest({ ...wire, command: 'format', changeId: manual.changeId, rangeLease, format: formats[0], uno: '.uno:Save' })).toBe(false)
  })

  it('binds renewed native lease and format state to the actual selection and sequence', () => {
    expect(nativeOfficeRangeLeaseSchema.safeParse(rangeLease).success).toBe(true)
    expect(isOfficeRangeLease(rangeLease)).toBe(true)
    const result = { ...wire, command: 'format', type: 'result', ok: true, state, selection, rangeLease, formatState }
    expect(isOfficeResult(result)).toBe(true)
    expect(nativeOfficeViewSchema.safeParse({ ...view, kind: 'docx', selection }).success).toBe(true)
    for (const next of [{ ...rangeLease, sequence: 1 }, { ...rangeLease, token: 'other_selection' }, { ...rangeLease, generation: -1 }, { ...rangeLease, path: '/private' }]) {
      expect(isOfficeResult({ ...result, rangeLease: next })).toBe(false)
    }
    expect(isOfficeResult({ ...result, formatState: { ...formatState, bold: true } })).toBe(false)
    expect(nativeOfficeViewSchema.safeParse({ ...view, kind: 'docx', selection: { ...selection, rangeLease: { ...rangeLease, sequence: 1 } } }).success).toBe(false)
  })

  it('correlates freeze to the same native operation and exact exported sequence', () => {
    const freezeAck = { changeId: manual.changeId, operationId: 'freeze_operation', generation: 3, sequence: 0 }
    expect(nativeOfficeFreezeAckSchema.safeParse(freezeAck).success).toBe(true)
    expect(isOfficeFreezeAck(freezeAck)).toBe(true)
    expect(isOfficeRequest({ ...wire, command: 'freeze', operationId: freezeAck.operationId, changeId: manual.changeId })).toBe(true)
    const result = { ...wire, command: 'freeze', operationId: freezeAck.operationId, type: 'result', ok: true, state: { ...state, manual: { ...state.manual, frozen: true } }, freezeAck }
    expect(isOfficeResult(result)).toBe(true)
    expect(isOfficeResult({ ...result, freezeAck: { ...freezeAck, operationId: 'other_operation' } })).toBe(false)
    expect(isOfficeResult({ ...result, freezeAck: { ...freezeAck, sequence: 1 } })).toBe(false)
    expect(isOfficeResult({ ...result, freezeAck: { ...freezeAck, changeId: 'd'.repeat(64) } })).toBe(false)
    expect(isOfficeResult({ ...result, freezeAck: { ...freezeAck, generation: 2 } })).toBe(false)
    expect(isOfficeRequest({ ...wire, command: 'export', changeId: manual.changeId, freezeAck })).toBe(true)
    expect(isOfficeRequest({ ...wire, command: 'export', changeId: 'd'.repeat(64), freezeAck })).toBe(false)
    expect(isOfficeRequest({ ...wire, command: 'export', freezeAck })).toBe(false)
    for (const error of ['native-input-unavailable', 'native-composition-pending']) expect(isOfficeResult({ ...wire, command: 'freeze', type: 'result', ok: false, error })).toBe(true)
    for (const error of ['native_input_unavailable', 'composition_pending', 'forbidden']) expect(nativeOfficeResponseSchema.safeParse({ ok: false, view: null, error }).success).toBe(true)
  })
})
