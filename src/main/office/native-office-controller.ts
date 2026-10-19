import { logNativeOfficeFailure, logNativeOfficeManualBeginFailure, logNativeOfficeSaveFailure, type NativeOfficeSaveFailureState, type NativeOfficeManualBeginFailureState, type NativeOfficeDiagnosticStage, type NativeOfficeDiagnosticErrorCode } from '../logger'
import { isOfficeResult, isOfficePreviewView, type OfficePreviewView, type OfficeRequestInput } from './office-protocol'
import { nativeOfficeCreateInputSchema, type NativeOfficeCreation, type NativeOfficeManualAuthority, nativeOfficeCommitInputSchema, nativeOfficeSelectionResponseSchema, nativeOfficeSelectionScopeSchema, type NativeOfficeSelectionResponse } from '../../../packages/runtime/src/contracts/native-office-editing'
import { createHash, randomUUID, randomBytes } from 'node:crypto'
import type { PresentOfficeSnapshot } from '../ipc/tool-result-local-display-ipc'
import { extname, isAbsolute, resolve } from 'node:path'
import { z } from 'zod'
import {
  pluginPackageHostResponseSchema, type PluginPackageHostRequest,
  type PluginPackageHostResponse, type PluginPackageView
} from '../../../packages/runtime/src/contracts/plugin-package-host'
import {
  objectEditingResponseSchema, type ObjectEditingResponse
} from '../../../packages/runtime/src/contracts/object-editing'
import {
  type NativeOfficeFreezeAck, nativeOfficeManualStateSchema, nativeOfficeActionMenuSchema, nativeOfficeFormatChoiceSchema, type NativeOfficeFormatChoice, type NativeOfficeActionMenu, nativeTypedPresentationSelection, nativeTypedWorkbookSelection, nativeOfficeRequestSchema, nativeOfficeSelectionSchema, type NativeOfficeBounds, type NativeOfficeAppearance,
  type NativeOfficeError, type NativeOfficeKind, type NativeOfficeResponse,
  type NativeOfficeSelection, type NativeOfficeView
} from '../../shared/native-office'

export const MAX_NATIVE_OFFICE_BYTES = 16 * 1024 * 1024
const MAX_BASE64_LENGTH = Math.ceil(MAX_NATIVE_OFFICE_BYTES / 3) * 4
const packageIds = { docx: 'analytix-documents', xlsx: 'analytix-spreadsheets', pptx: 'analytix-presentations' } as const
const operations = ['open-object', 'close-object'] as const
type HostOperation = typeof operations[number] | 'create-object' | 'begin-manual-edit' | 'plan-workbook-manual' | 'prepare-workbook-manual' | 'commit-object' | 'object-status' | 'capture-selection' | 'selection-read' | 'proposal-read' | 'proposal-accept' | 'proposal-reject' | 'selection-revoke' | 'object-recovery' | 'undo-change' | 'cancel-change' | 'resume-change' | 'annotation-read' | 'annotation-write'
const token = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/)
const digest = z.string().regex(/^[a-f0-9]{64}$/)
const sequence = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
const engineStateSchema = z.object({
  documentId: digest, version: digest, kind: z.enum(['docx', 'xlsx', 'pptx']),
  changeSequence: sequence, acknowledgedSequence: sequence, dirty: z.boolean(), manual: nativeOfficeManualStateSchema.optional()
}).strict().refine(s => s.acknowledgedSequence <= s.changeSequence && s.dirty === (s.changeSequence !== s.acknowledgedSequence))
type EngineState = z.infer<typeof engineStateSchema>
type HostResponseDiagnostic = Pick<NativeOfficeManualBeginFailureState, 'schemaAccepted' | 'outerCode' | 'innerCode'> &
  Partial<Pick<NativeOfficeSaveFailureState, 'outerSchemaAccepted' | 'innerSchemaAccepted' | 'receiptPresent' | 'receiptStatus'>>
type SaveDiagnostic = NativeOfficeSaveFailureState & Pick<NativeOfficeManualBeginFailureState, 'schemaAccepted'>
const eventBase = { documentId: digest, version: digest, operationId: token }
const engineEventSchema = z.discriminatedUnion('type', [
  z.object({ ...eventBase, type: z.literal('changed'), state: engineStateSchema }).strict(),
  z.object({ ...eventBase, type: z.literal('selection'), selection: nativeOfficeSelectionSchema }).strict(),
  z.object({ ...eventBase, type: z.literal('cell-edit-requested'), intent:z.enum(['f2','double-click']), selection: nativeOfficeSelectionSchema }).strict(),
  z.object({ ...eventBase, type: z.literal('save-requested') }).strict(),
  z.object({ ...eventBase, type: z.literal('view'), view: z.custom<OfficePreviewView>(value => isOfficePreviewView(value)).nullable() }).strict()
])

export type NativeOfficeEngineRequest = OfficeRequestInput
/** The isolated surface owns its channel and transport correlation. */
export interface NativeOfficeSurface {
  attach(bounds: NativeOfficeBounds, appearance?: NativeOfficeAppearance): void | Promise<void>
  hide(): void | Promise<void>
  request(envelope: NativeOfficeEngineRequest): Promise<unknown>
  destroy(): void | Promise<void>
  isPointerCurrent?(lease:NonNullable<NativeOfficeActionMenu['pointer']>): boolean
  presentPointerActions?(target:NativeOfficeActionMenu): Promise<string|null|NativeOfficeFormatChoice>
  dismissPointerActions?(lease:NonNullable<NativeOfficeActionMenu['pointer']>): void
}
export interface NativeOfficeControllerOptions {
  packageHost(request: PluginPackageHostRequest): Promise<PluginPackageHostResponse>
  createSurface(onEvent: (event: unknown) => void, protectedPresent?: boolean): NativeOfficeSurface | Promise<NativeOfficeSurface>
  onChange(view: NativeOfficeView | null): void
  newOperationId?: () => string
  freezeAnnotationInput?: (frozen: boolean) => Promise<boolean>
}

type Active = {
  view: NativeOfficeView; workspace: string; requestedPath: string; sessionId: string
  bounds: NativeOfficeBounds; appearance?: NativeOfficeAppearance; binding: PluginPackageView; surface: NativeOfficeSurface; openOperationId: string
  protectedSnapshot?: PresentOfficeSnapshot; closed?: boolean; abortPresent?: () => void
  viewControlEpoch?: number
  playbackAttempted?: boolean
  playbackStopUnknown?: boolean
  surfaceRetired?: boolean
  ready: boolean; failed: boolean; destroyed?: Promise<void>
  closePending?: boolean
  selections: Map<string, NativeOfficeSelection>
  appliedProposals: Set<string>
  revokedScopes: Set<string>
  decisionOperations: Map<string, string>
  pendingSave?: { operationId: string; sequence: number; revision: string; digest: string }
  threadId?: string
  approvedChange?: { changeId: string; threadId: string; saveOperationId: string; baseRevision: string; workbookManual?:true }
  manualAuthority?: NativeOfficeManualAuthority
  manualFreeze?: NativeOfficeFreezeAck
  manualExportAttempted?: boolean
  pendingManualCancel?: { changeId: string; threadId: string }
  pendingManualBegin?: { operationId: string; threadId: string; baseRevision: string }
  nativeWritable?: boolean
  pendingRecovery?: { operationId: string; revision: string; committed: boolean }
  annotationDesired?: { note: string; sourceRevision: string }
  annotationAttempt?: { expectedDraftRevision: string; note: string; sourceRevision: string }
}
type PreviewEngineRequest = NativeOfficeEngineRequest
type ProtectedOpenProgress = { stage: NativeOfficeDiagnosticStage; openDispatched: boolean; openResolved: boolean; guardPassed: boolean; owns: boolean }
class OfficeFailure extends Error {
  constructor(readonly code: NativeOfficeError, readonly transient = false) { super(code) }
}
function fail(code: NativeOfficeError, transient = false): never { throw new OfficeFailure(code, transient) }
function diagnosticError(code: NativeOfficeError): NativeOfficeDiagnosticErrorCode {
  return code === 'native_input_unavailable' || code === 'composition_pending' || code === 'presentation_unsupported' || code === 'presentation_failed' || code === 'presentation_start_timeout' ? 'engine_unavailable' : code
}
function saveDiagnostic(phase: NativeOfficeSaveFailureState['phase']): SaveDiagnostic {
  return {phase, schemaAccepted:null, outerSchemaAccepted:null, innerSchemaAccepted:null, outerCode:null, innerCode:null, receiptPresent:null, receiptStatus:null}
}
function recordSaveFailure(document: Pick<Active, 'view'>, error: unknown, state: NativeOfficeSaveFailureState): void {
  if (document.view.kind !== 'xlsx' || document.view.protectedPresent) return
  // Observability must never change the operation's result, including refusal.
  try {
    const { phase, outerSchemaAccepted, innerSchemaAccepted, outerCode, innerCode, receiptPresent, receiptStatus } = state
    logNativeOfficeSaveFailure(error instanceof OfficeFailure ? diagnosticError(error.code) : 'unexpected',
      { phase, outerSchemaAccepted, innerSchemaAccepted, outerCode, innerCode, receiptPresent, receiptStatus })
  } catch { /* Preserve the original failure. */ }
}
function hash(bytes: Uint8Array): string { return createHash('sha256').update(bytes).digest('hex') }
function decodeContent(content: string): Uint8Array {
  if (content.length === 0 || content.length > MAX_BASE64_LENGTH) fail('invalid_response')
  const bytes = Buffer.from(content, 'base64')
  if (bytes.byteLength === 0 || bytes.byteLength > MAX_NATIVE_OFFICE_BYTES || bytes.toString('base64') !== content) fail('invalid_response')
  return new Uint8Array(bytes)
}
function withoutChannel(value: unknown): unknown {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return value
  const copy = { ...value } as Record<string, unknown>
  delete copy.channel
  return copy
}

function workbookDiscussionText(selection: Extract<NativeOfficeSelection,{kind:'cells'}>): string {
  const column = (value:number) => {
    let label = ''
    for (value++; value; value = Math.floor((value-1)/26)) label = String.fromCharCode(65+(value-1)%26)+label
    return label
  }
  const ranges = selection.ranges.map(r => `${JSON.stringify(r.sheetName)}!${column(r.startColumn)}${r.startRow+1}:${column(r.endColumn)}${r.endRow+1}`).join('; ')
  const complete = !!selection.capture?.complete && !selection.capture.truncated
  return `Workbook selection: ${ranges}\nCapture: ${complete ? 'complete' : 'partial; values below are not the entire selection; do not infer whole-range totals'}; captured cells: ${selection.cells?.length ?? 0}; maximum 256. Read-only discussion reference; no write authority.\n${selection.text}`
}

/** Core-bound native sessions. At most two retained surfaces, only one visible. */
export function createNativeOfficeController(options: NativeOfficeControllerOptions) {
  let active: Active | undefined
  const documents = new Map<string, Active>()
  let visibilityEpoch = 0
  let visible = true
  let tail = Promise.resolve()
  let disposed = false
  let shutdown: Promise<void> | undefined
  let annotationInputFrozen: boolean | undefined = false
  const annotationFreezeHolds = new Set<symbol>()
  let freezeTail = Promise.resolve()
  let quitFreeze: Promise<() => Promise<void>> | undefined
  const syncAnnotationFreeze = () => {
    const result = freezeTail.then(async () => {
      while (annotationInputFrozen !== (annotationFreezeHolds.size > 0)) {
        const frozen = annotationFreezeHolds.size > 0
        let acknowledged = true
        try { if (options.freezeAnnotationInput) acknowledged = await options.freezeAnnotationInput(frozen) }
        catch { acknowledged = false }
        if (!acknowledged) {
          // A missing acknowledgement is not evidence of the renderer state.
          annotationInputFrozen = undefined
          fail('unavailable')
        }
        annotationInputFrozen = frozen
      }
    })
    freezeTail = result.catch(() => undefined)
    return result
  }
  async function holdAnnotationFreeze() {
    const hold = Symbol('annotation-close')
    annotationFreezeHolds.add(hold)
    try { await syncAnnotationFreeze() } catch (error) {
      annotationFreezeHolds.delete(hold)
      await syncAnnotationFreeze().catch(() => undefined)
      throw error
    }
    return async () => { annotationFreezeHolds.delete(hold); await syncAnnotationFreeze() }
  }
  async function freezeAnnotationInput(frozen: boolean) {
    if (frozen) {
      const pending = quitFreeze ??= holdAnnotationFreeze()
      try { await pending } catch (error) { if (quitFreeze === pending) quitFreeze = undefined; throw error }
    }
    else {
      const pending = quitFreeze; quitFreeze = undefined
      const release = await pending?.catch(() => undefined)
      if (release) await release()
      else await syncAnnotationFreeze()
    }
  }
  const newOperationId = () => {
    const id = (options.newOperationId ?? randomUUID)()
    if (!token.safeParse(id).success) fail('unavailable')
    return id
  }
  const getView = (): NativeOfficeView | null => active && !active.closed && (!active.protectedSnapshot || active.protectedSnapshot.isCurrent() && !active.protectedSnapshot.signal.aborted) ? structuredClone(active.view) : null
  const publish = () => { try { options.onChange(getView()) } catch { /* Observer failure must not change preview state. */ } }
  const response = (error?: NativeOfficeError): NativeOfficeResponse => ({ ok: !error, view: getView(), ...(error ? { error } : {}) })
  const setError = (document: Active, code: NativeOfficeError) => {
    if (active !== document) return
    document.view.status = 'error'
    document.view.error = code
    publish()
  }
  const serialize = (work: () => Promise<void>): Promise<NativeOfficeResponse> => {
    const result = tail.then(async () => {
      try { await work(); return response() }
      catch (error) { return response(error instanceof OfficeFailure ? error.code : 'unavailable') }
    })
    tail = result.then(() => undefined, () => undefined)
    return result
  }
  const requireActive = () => { if (!active || !active.ready) fail('unavailable'); return active }

  async function listBinding(kind: NativeOfficeKind, previous?: PluginPackageView): Promise<PluginPackageView> {
    let raw: unknown
    try { raw = await options.packageHost({ action: 'list' }) } catch { fail('package_unavailable') }
    const parsed = pluginPackageHostResponseSchema.safeParse(raw)
    if (!parsed.success || !parsed.data.ok || !('packages' in parsed.data)) fail('package_unavailable')
    if (new Set(parsed.data.packages.map(p => p.packageId)).size !== parsed.data.packages.length) fail('invalid_response')
    const binding = parsed.data.packages.find(p => p.packageId === packageIds[kind])
    if (!binding || !binding.available || !binding.materialized || binding.activationState !== 'recorded' || binding.desiredState !== 'enabled' || !operations.every(op => binding.operations.includes(op))) fail('package_unavailable')
    if (previous && binding.generationId !== previous.generationId) fail('package_changed')
    return binding
  }

  async function invokeOutput(document: { binding: PluginPackageView; view: Pick<NativeOfficeView, 'kind'> }, operation: HostOperation, input: Record<string, unknown>, diagnostic?: HostResponseDiagnostic): Promise<unknown> {
    // Relist only the same generation. This never enables or installs a plugin.
    document.binding = await listBinding(document.view.kind, document.binding)
    if (!document.binding.operations.includes(operation)) fail('package_unavailable')
    if (disposed && operation !== 'close-object') fail('unavailable')
    let raw: unknown
    try {
      raw = await options.packageHost({ action: 'invoke', packageId: document.binding.packageId,
        generationId: document.binding.generationId, expectedRevision: document.binding.activationRevision,
        contributionId: 'workspace-editor', operation, input })
    } catch { fail('unavailable') }
    const outer = pluginPackageHostResponseSchema.safeParse(raw)
    if (diagnostic) {
      diagnostic.schemaAccepted = outer.success
      if ('outerSchemaAccepted' in diagnostic) diagnostic.outerSchemaAccepted = outer.success
      if (outer.success && !outer.data.ok) diagnostic.outerCode = outer.data.code
    }
    if (!outer.success) fail('invalid_response')
    if (!outer.data.ok) fail('package_unavailable')
    if (!('output' in outer.data)) fail('invalid_response')
    return outer.data.output
  }
  async function invoke(document: { binding: PluginPackageView; view: Pick<NativeOfficeView, 'kind'> }, operation: HostOperation, input: Record<string, unknown>, diagnostic?: HostResponseDiagnostic): Promise<ObjectEditingResponse> {
    const inner = objectEditingResponseSchema.safeParse(await invokeOutput(document, operation, input, diagnostic))
    if (diagnostic) {
      diagnostic.schemaAccepted = inner.success
      diagnostic.innerSchemaAccepted = inner.success
      if (inner.success) {
        if (!inner.data.ok) diagnostic.innerCode = inner.data.code
        diagnostic.receiptPresent = 'receipt' in inner.data && !!inner.data.receipt
        diagnostic.receiptStatus = 'receipt' in inner.data && inner.data.receipt ? inner.data.receipt.status : null
      }
    }
    if (!inner.success) fail('invalid_response')
    return inner.data
  }

  async function invokeSelection(document: Active, operation: HostOperation, input: Record<string, unknown>, manualBegin?: NativeOfficeManualBeginFailureState): Promise<NativeOfficeSelectionResponse> {
    const result = nativeOfficeSelectionResponseSchema.safeParse(await invokeOutput(document, operation, input, manualBegin))
    if (manualBegin) {
      manualBegin.schemaAccepted = result.success
      if (result.success && !result.data.ok) manualBegin.innerCode = result.data.code
    }
    if (!result.success) fail('invalid_response')
    if (!result.data.ok) fail(result.data.code === 'forbidden' ? 'forbidden' : result.data.code === 'conflict' ? 'conflict' : result.data.code === 'scope_invalid' ? 'stale_selection' :
      ['plan-workbook-manual','prepare-workbook-manual'].includes(operation) && ['not_text','too_large','proposal_invalid','invalid_request'].includes(result.data.code) ? 'unsupported_selection' : 'unavailable')
    return result.data
  }

  const destroySurface = (document: Active): Promise<void> => {
    const surface = document.surface
    document.destroyed ??= Promise.resolve().then(async () => {
      await surface.destroy(); if (document.surface === surface) document.surfaceRetired = true
    }).catch(() => undefined)
    return document.destroyed
  }
  const retirePresent = (document: Active) => {
    if (!document.protectedSnapshot || document.closed) return
    document.closed = true; document.ready = false; document.failed = true
    document.protectedSnapshot.signal.removeEventListener('abort', document.abortPresent!)
    if (documents.get(document.view.objectId) === document) documents.delete(document.view.objectId)
    if (active === document) active = undefined
    void Promise.resolve(document.surface.hide()).catch(() => undefined)
    void destroySurface(document)
    publish()
  }
  async function guardPresent(document: Active, openProgress?: ProtectedOpenProgress) {
    const snapshot = document.protectedSnapshot
    if (!snapshot) return
    const owns = () => {
      const current = !disposed && !document.closed && documents.get(document.view.objectId) === document && !snapshot.signal.aborted && snapshot.isCurrent()
      if (openProgress) openProgress.owns = current
      return current
    }
    let valid = false
    try {
      if (owns()) {
        const guarded = await snapshot.guard()
        if (openProgress) openProgress.guardPassed = guarded
        valid = guarded && owns()
      }
    } catch { /* A lost cap cannot revive this instance. */ }
    if (!valid) { retirePresent(document); fail('unavailable') }
  }
  const invalidatePreview = (document: Active, code: NativeOfficeError) => {
    document.ready = false
    document.failed = true
    delete document.view.selection
    delete document.view.previewView
    if (document.view.playback && (document.playbackAttempted || document.view.playback.phase === 'running' || document.view.playback.phase === 'paused')) {
      // The last running sample is no longer a current confirmation. Preserve
      // its lifetime identity while projecting the actual failure, never idle.
      document.view.playback = {...document.view.playback,sequence:Math.min(Number.MAX_SAFE_INTEGER,document.view.playback.sequence + 1),
        phase:'error',slideIndex:null,slideCount:null}
    }
    setError(document, code)
    if (!document.view.dirty) void destroySurface(document)
  }
  function acceptState(document: Active, state: EngineState, expectedVersion = document.view.revision, endManual = false) {
    if (state.documentId !== document.view.objectId || state.version !== expectedVersion || state.kind !== document.view.kind ||
        state.changeSequence < (document.view.changeSequence ?? 0) || (!document.view.editing && !state.manual &&
          (state.dirty || !document.nativeWritable && (state.changeSequence !== 0 || state.acknowledgedSequence !== 0)))) fail('invalid_response')
    const authority = document.manualAuthority
    if (state.manual) {
      if (document.protectedSnapshot || state.kind !== 'docx' || !authority || state.manual.changeId !== authority.changeId ||
        state.manual.baseRevision !== authority.baseRevision || state.version !== authority.baseRevision ||
        document.view.manual && state.manual.generation !== document.view.manual.generation) fail('invalid_response')
      document.view.editing = true
      document.view.manual = structuredClone(state.manual)
    } else if (authority && !endManual) fail('invalid_response')
    else if (endManual) { delete document.view.manual; document.view.editing = false }
    if (document.view.revision !== state.version) { delete document.view.previewView; delete document.view.playback }
    document.view.changeSequence = state.changeSequence
    document.view.acknowledgedSequence = state.acknowledgedSequence
    document.view.dirty = state.dirty
    if (state.dirty) document.view.canUndo = false
    document.view.revision = state.version
    const scope = document.view.scope
    if (scope && (scope.changeSequence !== state.changeSequence || scope.baseRevision !== state.version) && !document.revokedScopes.has(scope.scopeId)) {
      document.revokedScopes.add(scope.scopeId)
      document.selections.clear()
      // Queue behind the current native operation, including an accepted
      // replacement, so its actual application is recorded before revocation.
      void serialize(async () => { await invokeSelection(document, 'selection-revoke', {sessionId:document.sessionId,scopeId:scope.scopeId}) })
    }
  }
  function acceptSelection(document: Active, selection: NativeOfficeSelection) {
    const expectedKind = { docx: 'text', xlsx: 'cells', pptx: 'shapes' }[document.view.kind]
    if (selection.documentId !== document.view.objectId || selection.version !== document.view.revision || selection.changeSequence !== (document.view.changeSequence ?? 0) ||
        (selection.kind !== expectedKind && selection.kind !== 'unavailable')) fail('invalid_response')
    document.view.selection = selection
    if (selection.token) { document.selections.set(selection.token, structuredClone(selection)); while (document.selections.size > 32) document.selections.delete(document.selections.keys().next().value!) }
  }
  function acceptPreviewView(document: Active, value: OfficePreviewView | null) {
    if (value === null) { delete document.view.previewView; return }
    if (!isOfficePreviewView(value, document.view.kind) || !value.readOnly && (!document.nativeWritable || document.view.kind !== 'docx' || !!document.protectedSnapshot) || (!document.view.editing && value.modified)) fail('invalid_response')
    if (value.playback) {
      const previous = document.view.playback
      if (previous && (value.playback.generation < previous.generation ||
          value.playback.generation === previous.generation && value.playback.sequence <= previous.sequence)) return
      document.view.playback = { ...value.playback }
      if (value.playback.phase === 'running' || value.playback.phase === 'paused') document.playbackAttempted = true
    }
    document.view.previewView = {zoom:value.zoom, fit:value.fit, page:value.page, pages:value.pages}
  }
  async function engine(document: Active, request: PreviewEngineRequest, current?: () => boolean, openProgress?: ProtectedOpenProgress) {
    if (disposed || document.failed || document.closed || document.closePending || documents.get(document.view.objectId) !== document) fail('engine_unavailable')
    if (openProgress) { openProgress.stage = 'controller_surface_open_guard_before'; openProgress.guardPassed = false }
    await guardPresent(document,openProgress)
    if (openProgress) { openProgress.stage = 'controller_surface_open'; openProgress.openDispatched = true }
    let raw: unknown
    try { raw = await document.surface.request(request) } catch (error) {
      const code = error instanceof Error ? error.message : ''
      // A canvas-owned start can occupy the surface outside Main's queue. This
      // refusal occurs before dispatching an edit; it is not engine corruption.
      if (['operation-in-progress','office-operation-in-progress'].includes(code)) fail('unavailable', true)
      if (request.command === 'local-view' && code === 'presentation-unsupported') fail('presentation_unsupported')
      if (request.command === 'local-view' && code === 'presentation-failed') fail('presentation_failed')
      if (request.command === 'local-view' && code === 'presentation-start-timeout') {
        // LO may still have an asynchronous view switch queued: end() before a
        // controller exists cannot prove cancellation. Retire this clean owner.
        document.playbackStopUnknown = true
        invalidatePreview(document, 'presentation_start_timeout')
        if (!document.view.dirty) await destroySurface(document)
        if (document.surfaceRetired) document.playbackStopUnknown = false
        fail('presentation_start_timeout')
      }
      if (request.command === 'local-view' && ['view-unavailable','unsupported-local-control','invalid-control-value','operation-in-progress','office-operation-in-progress'].includes(code)) fail('unavailable')
      if (code === 'native-input-unavailable') {
        // An explicit manual load may already have retired the readonly model.
        // Preserve Core recovery metadata, but never keep that sink usable.
        if (request.command === 'manual-edit') { invalidatePreview(document,'native_input_unavailable'); await destroySurface(document) }
        fail('native_input_unavailable')
      }
      if (code === 'native-composition-pending') fail('composition_pending')
      if (code === 'stale-selection') fail('stale_selection')
      if (['unsupported-selection', 'unsupported-format-fidelity', 'invalid-control-value', 'unsupported-command', 'command-unavailable', 'single-cell-required'].includes(code)) fail('unsupported_selection')
      if (code === 'export-too-large') fail('save_failed')
      if (code === 'unsaved-changes') fail('unsaved_changes')
      fail('engine_unavailable')
    }
    if (openProgress) openProgress.openResolved = true
    if (request.command === 'local-view' && !current?.()) fail('stale_selection')
    if (disposed || document.closed || documents.get(document.view.objectId) !== document) fail('engine_unavailable')
    if (openProgress) { openProgress.stage = 'controller_surface_open_guard_after'; openProgress.guardPassed = false }
    await guardPresent(document,openProgress)
    if (document.failed) fail('invalid_response')
    const correlated = raw && typeof raw === 'object' ? {...raw, channel:'main-controlled'} : raw
    if (!isOfficeResult(correlated) || !correlated.ok) fail('invalid_response')
    const value = correlated
    if (value.command !== request.command || value.operationId !== request.operationId || value.documentId !== request.documentId || value.version !== request.version) fail('invalid_response')
    if (request.command === 'close') {
      if (value.state || value.selection) fail('invalid_response')
      return
    }
    if (!value.state) fail('invalid_response')
    if (request.command === 'ack' && request.status === 'committed' &&
      (value.state.acknowledgedSequence !== request.exportedSequence || document.manualAuthority &&
        (value.state.changeSequence !== request.exportedSequence || value.state.dirty || !!value.state.manual))) fail('invalid_response')
    if (request.command === 'edit') {
      if (document.manualAuthority) fail('invalid_request')
      document.view.editing = true
    }
    if (request.command === 'manual-edit') {
      const manual = value.state.manual
      if (!request.manual || !manual || manual.changeId !== request.manual.changeId || manual.baseRevision !== request.manual.baseRevision || manual.frozen) fail('invalid_response')
      document.nativeWritable = true
    }
    const endManual = request.command === 'ack' && request.status === 'committed' && !!document.manualAuthority
    acceptState(document, value.state, request.command === 'ack' && request.status === 'committed' ? request.persistedVersion : document.view.revision, endManual)
    if (request.command === 'local-view') {
      if (!value.view || request.action?.startsWith('play-') && !value.view.playback) fail('invalid_response')
      acceptPreviewView(document, value.view)
    }
    if (request.command === 'captureSelection' && !value.selection) fail('invalid_response')
    if (value.selection && !document.protectedSnapshot) { const selection = nativeOfficeSelectionSchema.safeParse(value.selection); if (!selection.success) fail('invalid_response'); acceptSelection(document, selection.data) }
    return value
  }

  function isPlaying(document: Active): boolean {
    return document.view.playback?.phase === 'running' || document.view.playback?.phase === 'paused'
  }
  /** Retire the captured surface owner even after it lost visible/active ownership. */
  async function stopPlayback(document: Active, captured?: {surface:NativeOfficeSurface;objectId:string;revision:string;changeSequence:number}) {
    if (captured && document.surface !== captured.surface) return
    if (document.playbackStopUnknown && !document.surfaceRetired) fail('unknown')
    if (document.view.kind !== 'pptx' || !document.ready || document.failed || document.closed || document.closePending) return
    const owner = captured ?? {surface:document.surface,objectId:document.view.objectId,revision:document.view.revision,changeSequence:document.view.changeSequence ?? 0}
    const {surface,objectId,revision,changeSequence} = owner
    // A recovery replacement destroys its predecessor before installing the new
    // surface. A queued hide must never stop that replacement's lifetime.
    if (document.surface !== surface) return
    const current = () => !disposed && !document.closed && !document.closePending && !document.failed &&
      document.surface === surface && documents.get(objectId) === document && document.view.revision === revision &&
      (document.view.changeSequence ?? 0) === changeSequence
    if (!current()) return
    try {
      // A hidden in-flight start can have no published running state. Read the
      // same native owner after the serialized command, never infer it stopped.
      await engine(document, {command:'local-view',action:'play-read',operationId:newOperationId(),documentId:objectId,version:revision}, current)
      if (!current()) fail('stale_selection')
      const playback = document.view.playback
      if (playback?.phase === 'idle' || playback?.phase === 'unsupported') document.playbackAttempted = false
      if (isPlaying(document) || document.playbackAttempted || playback?.phase === 'error') {
        await engine(document, {command:'local-view',action:'play-end',playbackGeneration:playback?.generation ?? 0,
          operationId:newOperationId(),documentId:objectId,version:revision}, current)
      }
      document.playbackAttempted = false
      document.playbackStopUnknown = false
    } catch (error) {
      const code = error instanceof OfficeFailure ? error.code : 'engine_unavailable'
      // Missing presentation capability before any start does not invalidate a
      // working readonly preview. An unknown live show must lose its surface.
      if (code === 'presentation_unsupported' && !document.playbackAttempted && !isPlaying(document)) return
      document.playbackStopUnknown = true
      invalidatePreview(document, code)
      if (!document.view.dirty) await destroySurface(document)
      if (document.surfaceRetired) document.playbackStopUnknown = false
      throw error
    }
  }

  function onEvent(document: Active, raw: unknown) {
    if (disposed || documents.get(document.view.objectId) !== document || document.closed || document.failed || document.closePending) return
    if (document.protectedSnapshot) {
      if (!document.protectedSnapshot.isCurrent() || document.protectedSnapshot.signal.aborted) { retirePresent(document); return }
      const eventEpoch = visibilityEpoch
      const check = engineEventSchema.safeParse(withoutChannel(raw))
      if (check.success && (check.data.type === 'selection' || check.data.type === 'save-requested')) return
      void guardPresent(document).then(() => {
        if (disposed || document.closed || documents.get(document.view.objectId) !== document) return
        if (!check.success) { invalidatePreview(document, 'invalid_response'); return }
        const event = check.data
        if (event.documentId !== document.view.objectId || event.version !== document.view.revision || event.operationId !== document.openOperationId) return
        try {
          if (event.type === 'changed') acceptState(document, event.state)
          if (event.type === 'view') {
            if (active !== document || !visible || !document.ready || eventEpoch !== visibilityEpoch ||
              document.viewControlEpoch !== undefined && document.viewControlEpoch !== visibilityEpoch) return
            acceptPreviewView(document, event.view)
          }
          publish()
        } catch { invalidatePreview(document, 'invalid_response') }
      }).catch(() => undefined)
      return
    }
    const fatal = z.object({ type: z.literal('fatal'), code: z.enum(['office-startup-failed', 'office-assets-unavailable', 'office-protocol-invalid', 'office-operation-timeout-unknown', 'office-engine-closed-unknown']) }).strict().safeParse(raw)
    if (fatal.success) { invalidatePreview(document, 'engine_unavailable'); return }
    const value = withoutChannel(raw)
    // Stale/foreign-document events cannot change the active preview.
    if (value && typeof value === 'object' && 'documentId' in value && 'version' in value &&
        (value.documentId !== document.view.objectId || value.version !== document.view.revision)) return
    const parsed = engineEventSchema.safeParse(value)
    if (!parsed.success) { invalidatePreview(document, 'invalid_response'); return }
    const event = parsed.data
    if (event.type === 'changed') {
      try { acceptState(document, event.state); delete document.view.selection; publish() } catch { invalidatePreview(document, 'invalid_response') }
      return
    }
    if (event.operationId !== document.openOperationId) return
    if (event.type === 'view') {
      if (!document.ready || active !== document || !visible ||
        (document.viewControlEpoch !== undefined && document.viewControlEpoch !== visibilityEpoch)) return
      try { acceptPreviewView(document, event.view); publish() } catch { invalidatePreview(document, 'invalid_response') }
      return
    }
    if (event.type === 'save-requested') { if (document.view.editing) void serialize(() => save(document)); return }
    if (event.type !== 'selection' && event.type !== 'cell-edit-requested') return
    try {
      const selection = event.type === 'cell-edit-requested' && document.view.kind === 'xlsx' && event.selection.kind === 'cells' && event.selection.token
        ? {...event.selection,editIntent:{kind:event.intent,id:event.selection.token}} : event.selection
      acceptSelection(document, selection); publish()
    }
    catch { invalidatePreview(document, 'invalid_response') }
  }

  async function open(request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, { action: 'open' }>, epoch: number) {
    let diagnosticStage: NativeOfficeDiagnosticStage = 'controller_validation'
    let diagnosticLogged = false
    try {
      if (!isAbsolute(request.workspace)) fail('invalid_request')
      const kind = extname(request.path).slice(1).toLowerCase() as NativeOfficeKind
      if (!Object.hasOwn(packageIds, kind)) fail('invalid_request')
      if (active) await stopPlayback(active)
      const closing = [...documents.values()].find(d => d.closePending && d.workspace === request.workspace && resolve(d.workspace, d.requestedPath) === resolve(request.workspace, request.path))
      if (closing) await close(closing)
      const retained = [...documents.values()].find(d => !d.protectedSnapshot && d.workspace === request.workspace && resolve(d.workspace, d.requestedPath) === resolve(request.workspace, request.path) && d.ready)
      if (retained) {
        if (active !== retained) await active?.surface.hide()
        active = retained
      }
      if (active) {
        if (!active.protectedSnapshot && active.ready && active.workspace === request.workspace && resolve(active.workspace, active.requestedPath) === resolve(request.workspace, request.path)) {
          diagnosticStage = 'controller_package'
          active.binding = await listBinding(active.view.kind, active.binding)
          active.bounds = request.bounds; active.appearance = request.appearance
          diagnosticStage = 'controller_recovery'
          if (active.pendingRecovery) fail('unknown')
          await recover(active, request.threadId ?? undefined)
          diagnosticStage = 'controller_surface_attach'
          if (visible && epoch === visibilityEpoch) await active.surface.attach(request.bounds, request.appearance)
          active.view.status = 'ready'
          delete active.view.error
          publish()
          return
        }
        await active.surface.hide()
        if (active.closePending || !active.ready && !active.view.dirty) await close(active, true)
      }
      if (documents.size >= 2) {
        const clean = [...documents.values()].find(d => !d.view.dirty && !d.pendingSave && !d.pendingRecovery && d !== retained)
        if (!clean) fail('capacity')
        await close(clean)
      }
      diagnosticStage = 'controller_package'
      const binding = await listBinding(kind)
      let surface: NativeOfficeSurface | undefined
      let document: Active | undefined
      let cleanup: { binding: PluginPackageView; view: Pick<NativeOfficeView, 'kind'>; sessionId: string } | undefined
      try {
        // A temporary Main-owned holder lets every Core invoke use the same
        // current binding validation without exposing session state to Renderer.
        const holder = { binding, view: { kind } }
        diagnosticStage = 'controller_open_object'
        const result = await invoke(holder, 'open-object', { object: { workspace: request.workspace, path: request.path } })
        if (!result.ok) fail(result.code === 'forbidden' ? 'forbidden' : 'unavailable')
        if (!('document' in result)) fail('unavailable')
        const opened = result.document
        cleanup = { ...holder, sessionId: opened.sessionId }
        if (epoch !== visibilityEpoch || !visible) fail('unavailable')
        diagnosticStage = 'controller_content'
        const bytes = decodeContent(opened.content)
        if (!isAbsolute(opened.path) || resolve(opened.path) !== resolve(request.workspace, request.path) || opened.revision !== hash(bytes) || extname(opened.path).toLowerCase() !== '.' + kind) fail('invalid_response')
        if (disposed) fail('unavailable')
        diagnosticStage = 'controller_surface_create'
        surface = await options.createSurface(event => { if (document && document.surface === surface) onEvent(document, event) })
        if (disposed || epoch !== visibilityEpoch || !visible) fail('unavailable')
        document = { view: { objectId: opened.objectId, path: opened.path, kind, revision: opened.revision, dirty: false, status: 'loading', ...(kind === 'xlsx' ? {workbookEditing:result.workbookEditing ?? {editable:false,reason:'unsupported_workbook' as const}} : {}) },
          workspace: request.workspace, requestedPath: request.path, sessionId: opened.sessionId,
          bounds:request.bounds, appearance:request.appearance, binding: holder.binding, selections:new Map(), appliedProposals:new Set(), revokedScopes:new Set(), decisionOperations:new Map(), surface, openOperationId: newOperationId(), ready: false, failed: false }
        documents.set(document.view.objectId, document)
        active = document
        publish()
        diagnosticStage = 'controller_surface_attach'
        await surface.attach(request.bounds, request.appearance)
        diagnosticStage = 'controller_surface_open'
        await engine(document, { command: 'open', documentId: opened.objectId, version: opened.revision, operationId: document.openOperationId, kind, bytes })
        document.ready = true
        diagnosticStage = 'controller_recovery'
        await recover(document, request.threadId ?? undefined)
        document.view.status = 'ready'
        publish()
      } catch (error) {
        logNativeOfficeFailure(diagnosticStage, error instanceof OfficeFailure ? diagnosticError(error.code) : 'unexpected', false)
        diagnosticLogged = true
        if (document) await destroySurface(document)
        else { try { await surface?.destroy() } catch { /* Failed preview owns no usable surface. */ } }
        if (cleanup) {
          try { await invoke(cleanup, 'close-object', { sessionId: cleanup.sessionId }) } catch { /* Best-effort cleanup of a failed open only. */ }
        }
        if (document && documents.get(document.view.objectId) === document) documents.delete(document.view.objectId)
        if (active === document) active = undefined
        publish()
        throw error
      }
    } catch (error) {
      if (!diagnosticLogged) logNativeOfficeFailure(diagnosticStage, error instanceof OfficeFailure ? diagnosticError(error.code) : 'unexpected', false)
      throw error
    }
  }

  async function openProtected(request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, {action:'openPresent'}>, snapshot: PresentOfficeSnapshot, epoch: number) {
    let diagnosticStage: NativeOfficeDiagnosticStage = 'controller_validation'
    let diagnosticLogged = false
    try {
      const current = () => !disposed && visible && epoch === visibilityEpoch && !snapshot.signal.aborted && snapshot.isCurrent()
      if (!current() || snapshot.bytes.byteLength === 0 || snapshot.bytes.byteLength > MAX_NATIVE_OFFICE_BYTES || snapshot.truncated || hash(snapshot.bytes) !== snapshot.digest || !['docx','pptx'].includes(snapshot.kind)) fail('unavailable')
      if (!await snapshot.guard() || !current()) fail('unavailable')
      diagnosticStage = 'controller_package'
      const binding = await listBinding(snapshot.kind)
      if (!current()) fail('unavailable')
      if (active) await stopPlayback(active)
      if (!current()) fail('unavailable')
      if (active?.protectedSnapshot) retirePresent(active)
      else await active?.surface.hide()
      if (documents.size >= 2) {
        const clean = [...documents.values()].find(d => !d.view.dirty && !d.pendingSave && !d.pendingRecovery && !d.annotationDesired && !d.annotationAttempt)
        if (!clean) fail('capacity')
        await close(clean)
      }
      let surface: NativeOfficeSurface | undefined, document: Active | undefined
      let openProgress: ProtectedOpenProgress | undefined
      try {
        diagnosticStage = 'controller_surface_create'
        surface = await options.createSurface(event => { if (document && document.surface === surface) onEvent(document, event) }, true)
        if (!current() || !await snapshot.guard() || !current()) fail('unavailable')
        const objectId = randomBytes(32).toString('hex')
        document = { view:{objectId,path:snapshot.path,kind:snapshot.kind,revision:snapshot.digest,dirty:false,status:'loading',protectedPresent:structuredClone(snapshot.selector)},
          workspace:snapshot.workspaceRoot,requestedPath:snapshot.path,sessionId:'',binding,surface,
          bounds:request.bounds,appearance:request.appearance,selections:new Map(),appliedProposals:new Set(),revokedScopes:new Set(),decisionOperations:new Map(),
          openOperationId:newOperationId(),ready:false,failed:false,protectedSnapshot:snapshot }
        documents.set(objectId, document); active = document
        document.abortPresent = () => retirePresent(document!)
        snapshot.signal.addEventListener('abort', document.abortPresent, {once:true})
        await guardPresent(document); publish()
        diagnosticStage = 'controller_surface_attach'
        await surface.attach(request.bounds,request.appearance)
        await guardPresent(document)
        openProgress = {stage:'controller_surface_open',openDispatched:false,openResolved:false,guardPassed:false,owns:false}
        await engine(document,{command:'open',documentId:objectId,version:snapshot.digest,kind:snapshot.kind,bytes:snapshot.bytes,operationId:document.openOperationId},undefined,openProgress)
        openProgress.stage = 'controller_surface_open_guard_final'; openProgress.guardPassed = false
        await guardPresent(document,openProgress)
        openProgress.stage = 'controller_surface_open_current_final'
        if (!current()) fail('unavailable')
        document.ready = true; document.view.status = 'ready'; publish()
      } catch (error) {
        // Capture only closed state before this catch performs cleanup. Diagnostic
        // observation never repeats the asynchronous authority/file guard; owns
        // and guardPassed retain their last checks before guard-driven retirement.
        let snapshotCurrent = false
        if (openProgress) { try { snapshotCurrent = snapshot.isCurrent() } catch { /* Unknown currentness stays false. */ } }
        const openState = openProgress ? {openDispatched:openProgress.openDispatched,openResolved:openProgress.openResolved,guardPassed:openProgress.guardPassed,
          owns:openProgress.owns,
          visible,epochMatch:epoch === visibilityEpoch,snapshotCurrent,ownerAborted:snapshot.signal.aborted} : undefined
        logNativeOfficeFailure(openProgress?.stage ?? diagnosticStage, error instanceof OfficeFailure ? diagnosticError(error.code) : 'unexpected', true, openState)
        diagnosticLogged = true
        if (document) { retirePresent(document); await destroySurface(document) }
        else { try { await surface?.destroy() } catch { /* A failed private preview owns no document. */ } }
        throw error
      }
    } catch (error) {
      if (!diagnosticLogged) logNativeOfficeFailure(diagnosticStage, error instanceof OfficeFailure ? diagnosticError(error.code) : 'unexpected', true)
      throw error
    }
  }

  async function createObject(request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, {action:'create'}>): Promise<NativeOfficeResponse> {
    let creation: NativeOfficeCreation | undefined
    const result = await serialize(async () => {
      const holder = {binding:await listBinding('docx'),view:{kind:'docx' as const}}
      const input = nativeOfficeCreateInputSchema.parse({threadId:request.threadId,operationId:request.operationId,fileName:request.fileName})
      let raw: unknown
      try { raw = await invokeOutput(holder,'create-object',input) }
      catch (error) {
        // A lost creation reply is queried/replayed with this same caller op.
        if (error instanceof OfficeFailure && error.code === 'unavailable') fail('unknown')
        throw error
      }
      const parsed = nativeOfficeSelectionResponseSchema.safeParse(raw)
      if (!parsed.success) fail('invalid_response')
      if (!parsed.data.ok) fail(parsed.data.code === 'forbidden' ? 'forbidden' : parsed.data.code === 'conflict' ? 'conflict' : 'unavailable')
      if (!('creation' in parsed.data)) fail('invalid_response')
      const created = parsed.data.creation
      if (created.operationId !== request.operationId || !isAbsolute(created.workspace) || !isAbsolute(created.path) ||
        resolve(created.workspace,request.fileName) !== created.path) fail('invalid_response')
      // Creation persists only the Core receipt. Renderer navigation owns
      // whether this file is still current and selects it before Panel opens it.
      // A late receipt must never create, attach or select a native surface.
      creation = created
    })
    return {...result,view:null,...(creation ? {creation} : {})}
  }

  function manualBeginFailureState(document: Active): NativeOfficeManualBeginFailureState {
    return {schemaAccepted:null,outerCode:null,innerCode:null,
      pendingSave:!!document.pendingSave,pendingRecovery:!!document.pendingRecovery,pendingManualCancel:!!document.pendingManualCancel}
  }
  async function beginManualEdit(document: Active, threadId: string) {
    if (document.view.kind !== 'docx' || document.protectedSnapshot || threadId !== document.threadId) fail('invalid_request')
    const diagnostic = manualBeginFailureState(document)
    if (document.pendingSave || document.pendingRecovery || document.pendingManualCancel) {
      logNativeOfficeManualBeginFailure('unknown',diagnostic)
      fail('unknown')
    }
    if (document.view.manual && document.manualAuthority) {
      if (document.view.manual.frozen) fail('unsaved_changes')
      return
    }
    if (document.view.dirty || document.approvedChange && !document.manualAuthority) fail('unsaved_changes')
    document.binding = await listBinding('docx', document.binding)
    if (!document.binding.operations.includes('begin-manual-edit')) fail('package_unavailable')
    const attempt = document.pendingManualBegin ??= {operationId:newOperationId(),threadId,baseRevision:document.view.revision}
    if (attempt.threadId !== threadId || attempt.baseRevision !== document.view.revision) fail('conflict')
    if (!document.manualAuthority) {
      let result: NativeOfficeSelectionResponse
      try { result = await invokeSelection(document,'begin-manual-edit',{sessionId:document.sessionId,...attempt},diagnostic) }
      catch (error) {
        logNativeOfficeManualBeginFailure(error instanceof OfficeFailure ? diagnosticError(error.code) : 'unexpected',diagnostic)
        // Replay this exact begin identity after a lost reply. Another begin
        // could reserve a second Core transaction without settling the first.
        if (error instanceof OfficeFailure && error.code === 'unavailable') fail('unknown')
        throw error
      }
      if (!result.ok || !('manual' in result) || result.manual.baseRevision !== attempt.baseRevision) fail('invalid_response')
      document.manualAuthority = result.manual
      document.approvedChange = {changeId:result.manual.changeId,threadId,saveOperationId:result.manual.saveOperationId,baseRevision:result.manual.baseRevision}
      delete document.view.scope; delete document.view.proposals; delete document.view.localReviews
      document.selections.clear()
    }
    await engine(document,{command:'manual-edit',operationId:newOperationId(),documentId:document.view.objectId,
      version:document.view.revision,manual:document.manualAuthority})
    delete document.pendingManualBegin
    document.view.status = 'ready'; delete document.view.error; publish()
  }
  async function formatManual(document: Active, request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>,{action:'format'}>) {
    if (document.pendingManualCancel) fail('unknown')
    if (!document.manualAuthority || !document.view.manual || document.view.manual.frozen || document.pendingSave || document.pendingRecovery ||
      request.changeId !== document.manualAuthority.changeId || request.rangeLease.generation !== document.view.manual.generation ||
      request.rangeLease.sequence !== document.view.changeSequence) fail('stale_selection')
    const formatted = await engine(document,{command:'format',operationId:newOperationId(),documentId:document.view.objectId,
      version:document.view.revision,changeId:request.changeId,rangeLease:request.rangeLease,format:request.format})
    if (!formatted?.rangeLease || !formatted.formatState || !formatted.selection) fail('invalid_response')
    document.view.status = 'ready'; delete document.view.error; publish()
  }

  async function editWorkbookCell(document: Active, request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>,{action:'editWorkbookCell'}>) {
    if (document.view.kind !== 'xlsx' || document.protectedSnapshot || document.threadId !== request.threadId ||
      !document.view.workbookEditing?.editable) fail('unsupported_selection')
    if (document.pendingSave || document.pendingRecovery || document.pendingManualCancel) fail('unknown')
    if (document.view.dirty || document.approvedChange || document.manualAuthority) fail('unsaved_changes')
    const selected = document.view.selection
    const original = selected && nativeTypedWorkbookSelection(selected)
    if (!selected || selected.token !== request.selectionToken || !original || original.cells.length !== 1 ||
      selected.changeSequence !== request.expectedChangeSequence || selected.version !== request.revision) fail('stale_selection')
    await recover(document,request.threadId)
    if (document.view.recovery?.pending) fail('unsaved_changes')
    const cell = original.cells[0], writeTarget = {sheet:cell.sheet,column:cell.column,row:cell.row}
    const coreTarget = {...writeTarget,sheetName:original.sheetName}
    const input = {sessionId:document.sessionId,threadId:request.threadId,baseRevision:request.revision,writeTarget:coreTarget,edit:request.edit}
    // Core's existing parser determines the bounded read scope. It conveys no
    // write authority; the native token continues to name the original cell.
    const plan = await invokeSelection(document,'plan-workbook-manual',input)
    if (!('dependencyBounds' in plan) || plan.dependencyBounds.sheet !== writeTarget.sheet) fail('invalid_response')
    await engine(document,{command:'edit',operationId:newOperationId(),documentId:document.view.objectId,version:request.revision})
    const current = document.view.selection, target = current && nativeTypedWorkbookSelection(current)
    if (!current?.token || !target || JSON.stringify(target) !== JSON.stringify(original)) fail('stale_selection')
    const captured = await engine(document,{command:'captureCells',operationId:newOperationId(),documentId:document.view.objectId,
      version:request.revision,selectionToken:current.token,expectedChangeSequence:request.expectedChangeSequence,bounds:plan.dependencyBounds})
    if (!captured?.workbook) fail('invalid_response')
    let prepared: NativeOfficeSelectionResponse
    try {
      prepared = await invokeSelection(document,'prepare-workbook-manual',{...input,operationId:newOperationId(),selectionToken:current.token,
        changeSequence:request.expectedChangeSequence,workbook:captured.workbook})
    } catch (error) {
      // A lost prepare reply may own a durable reservation. Discover it before
      // another input; never synthesize a proposal or retry a new operation.
      try { await recover(document,request.threadId) } catch { /* Preserve the original failure. */ }
      if (error instanceof OfficeFailure && error.code === 'unavailable') fail('unknown')
      throw error
    }
    if (!('workbookManual' in prepared)) fail('invalid_response')
    const manual = prepared.workbookManual
    if (manual.baseRevision !== request.revision || JSON.stringify(manual.workbook.before) !== JSON.stringify(captured.workbook)) fail('invalid_response')
    document.approvedChange = {changeId:manual.changeId,threadId:request.threadId,baseRevision:manual.baseRevision,saveOperationId:manual.saveOperationId,workbookManual:true}
    document.view.canUndo = false
    delete document.view.scope; delete document.view.proposals; delete document.view.localReviews
    try {
      await engine(document,{command:'replaceCells',operationId:newOperationId(),documentId:document.view.objectId,version:request.revision,
        selectionToken:current.token,expectedChangeSequence:request.expectedChangeSequence,workbook:manual.workbook,writeTarget})
    } catch (error) {
      recordSaveFailure(document,error,saveDiagnostic('replace'))
      if (error instanceof OfficeFailure && error.transient) throw error
      await restoreFailedWorkbook(document); fail('save_failed')
    }
    await save(document)
    const committed = document.view.recovery?.current
    if (!committed || !('version' in committed) || committed.version !== 3 || committed.origin !== 'manual' ||
      committed.changeId !== manual.changeId || committed.status !== 'committed' || committed.baseRevision !== request.revision ||
      committed.revision !== document.view.revision || committed.revision === request.revision || document.view.dirty || document.pendingSave || document.pendingRecovery) fail('unknown')
  }

  async function freezeManual(document: Active): Promise<NativeOfficeFreezeAck> {
    const manual = document.manualAuthority, current = document.view.manual
    if (!manual || !current || current.changeId !== manual.changeId || current.baseRevision !== manual.baseRevision || current.baseRevision !== document.view.revision) fail('invalid_response')
    if (current.frozen) {
      const proof = document.manualFreeze
      if (!proof || proof.changeId !== manual.changeId || proof.generation !== current.generation || proof.sequence !== document.view.changeSequence) fail('invalid_response')
      return structuredClone(proof)
    }
    if (document.manualFreeze) fail('invalid_response')
    const operationId = newOperationId()
    const frozen = await engine(document,{command:'freeze',operationId,documentId:document.view.objectId,version:document.view.revision,changeId:manual.changeId})
    const proof = frozen?.freezeAck
    if (!proof || proof.operationId !== operationId || proof.changeId !== manual.changeId || proof.generation !== document.view.manual?.generation ||
      proof.sequence !== document.view.changeSequence || !document.view.manual?.frozen) fail('invalid_response')
    document.manualFreeze = structuredClone(proof)
    return structuredClone(proof)
  }

  async function close(document = active, discard = false) {
    if (!document) return
    if (document.protectedSnapshot) {
      try { await stopPlayback(document) } finally { retirePresent(document); await destroySurface(document) }
      return
    }
    const releaseFreeze = await holdAnnotationFreeze()
    try {
      await flushAnnotation(document)
      if (document.pendingSave || document.pendingRecovery || document.pendingManualBegin) fail('unknown')
      if (document.manualAuthority) {
        if (!discard && !document.pendingManualCancel) {
          if (document.view.dirty) fail('unsaved_changes')
          // Cached dirty=false is not proof that Qt/composition has no input.
          // Keep the exact proof if new changes are discovered so an explicit
          // save can use that prefix without minting another freeze identity.
          await freezeManual(document)
          if (document.view.dirty) fail('unsaved_changes')
        }
        await cancelChange(document,document.threadId!,document.manualAuthority.changeId,false)
        return
      }
      if (!document.closePending) {
        if (document.view.dirty && !discard) fail('unsaved_changes')
        if (document.playbackStopUnknown && discard) {
          // The ordinary discard guard has passed. Retire the unconfirmed
          // engine before Core cleanup; never erase dirty state to unlock it.
          await destroySurface(document)
          if (!document.surfaceRetired) fail('unknown')
          document.playbackStopUnknown = false
        } else await stopPlayback(document)
        if (document.ready && !document.failed) {
          try { await engine(document, { command: 'close', documentId: document.view.objectId, version: document.view.revision,
            operationId: newOperationId(), expectedChangeSequence: document.view.changeSequence ?? 0, discard }) } catch (error) { if (document.view.dirty && !discard) throw error }
        }
        // The original close already passed its dirty/discard checks. Retain
        // this owner solely to retry Core cleanup, never to reopen its engine.
        document.closePending = true
        document.ready = false
      }
      try {
        const result = await invoke(document, 'close-object', { sessionId: document.sessionId })
        if (!result.ok || !('closed' in result)) fail('unavailable')
      } catch (error) {
        document.view.status = 'error'
        document.view.error = error instanceof OfficeFailure ? error.code : 'unavailable'
        publish()
        throw error
      } finally {
        await destroySurface(document)
      }
      if (documents.get(document.view.objectId) === document) documents.delete(document.view.objectId)
      if (active === document) active = undefined
      publish()
    } finally { await releaseFreeze() }
  }
  async function settleSave(document: Active, result: ObjectEditingResponse, diagnostic?: SaveDiagnostic) {
    const pending = document.pendingSave
    if (!pending || !('receipt' in result) || !result.receipt) fail('unknown')
    const receipt = result.receipt
    if (receipt.operationId !== pending.operationId) fail('invalid_response')
    if (receipt.status === 'unknown' || receipt.status === 'pending') fail('unknown')
    if (receipt.status === 'committed' && receipt.revision !== pending.digest) fail('invalid_response')
    if (receipt.status === 'committed') {
      delete document.view.scope; delete document.view.proposals; delete document.view.localReviews; document.selections.clear()
    }
    const receiptPhase = diagnostic?.phase
    if (diagnostic) diagnostic.phase = 'ack'
    try {
      await engine(document, {command:'ack', documentId:document.view.objectId, version:pending.revision,
        operationId:newOperationId(), exportOperationId:pending.operationId, exportedSequence:pending.sequence,
        status:receipt.status === 'committed' ? 'committed' : 'conflict',
        ...(receipt.status === 'committed' ? {persistedVersion:receipt.revision} : {})})
    } catch (error) {
      if (receipt.status !== 'committed' || !(document.manualAuthority || document.approvedChange?.workbookManual)) throw error
      if (diagnostic) { recordSaveFailure(document,error,diagnostic); diagnostic.phase = 'recovery' }
      // The exact Core receipt already proves this artifact was committed. A
      // missing native ACK (including failed virtual-input synchronization)
      // retires that sink; it never licenses another commit or a fabricated ACK.
      // Existing recovery rereads Core bytes and validates the receipt revision
      // before constructing a fresh readonly model.
      document.pendingRecovery = {operationId:pending.operationId,revision:receipt.revision,committed:true}
      document.view.saving = true; publish()
      await finishRecovery(document)
      return
    }
    if (diagnostic && receiptPhase) diagnostic.phase = receiptPhase
    document.pendingSave = undefined; document.view.saving = false
    if (receipt.status === 'conflict') { setError(document, 'conflict'); fail('conflict') }
    delete document.approvedChange; delete document.manualAuthority; delete document.pendingManualBegin
    delete document.manualFreeze; delete document.manualExportAttempted; delete document.pendingManualCancel
    if (diagnostic) diagnostic.phase = 'recovery'
    await recover(document, document.threadId)
    document.view.status = 'ready'; delete document.view.error; publish()
  }
  async function save(document: Active) {
    const diagnostic = document.view.kind === 'xlsx' ? saveDiagnostic('prepare') : undefined
    try { await saveAttempt(document,diagnostic) }
    catch (error) { if (diagnostic) recordSaveFailure(document,error,diagnostic); throw error }
  }
  async function saveAttempt(document: Active, diagnostic?: SaveDiagnostic) {
    if (document.closePending) fail('unavailable')
    if (document.pendingRecovery) {
      if (diagnostic) diagnostic.phase = 'recovery'
      const pending = document.pendingRecovery
      try { await finishRecovery(document) } catch (error) {
        try {
          await recover(document, document.threadId)
          const state = document.view.recovery
          const retrySave = state?.pending?.canResume && state.pending.saveOperationId === pending.operationId
          const retryUndo = state?.current?.canRetryUndo && state.current.undoOperationId === pending.operationId
          if (document.pendingRecovery === pending && (retrySave || retryUndo)) {
            // Query only exposes a new explicit continuation. It never performs
            // it, nor claims that the earlier attempt could not have committed.
            delete document.pendingRecovery
            document.view.saving = !!document.pendingSave
            publish()
          }
        } catch { /* Keep the unresolved operation discoverable for another query. */ }
        throw error
      }
      return
    }
    if (!document.view.editing) fail('invalid_request')
    if (document.pendingSave) {
      if (diagnostic) diagnostic.phase = 'query'
      try {
        await settleSave(document, await invoke(document, 'object-status', {sessionId:document.sessionId, operationId:document.pendingSave.operationId},diagnostic),diagnostic)
      } catch (error) {
        // The journal may not exist yet even though Core retained the candidate.
        // Discover explicit recovery actions without retrying the file write.
        try { await recover(document, document.threadId) } catch { /* Preserve the original uncertain outcome. */ }
        throw error
      }
      return
    }
    if (document.pendingManualCancel) {
      await cancelChange(document,document.pendingManualCancel.threadId,document.pendingManualCancel.changeId)
      return
    }
    const manual = document.manualAuthority
    // Local dirty notifications can trail native keys, composition or paste.
    // A manual save must establish the native prefix before deciding it is a
    // no-op. A previously attempted export keeps its owner until discard; a
    // close that discovered input may reuse its exact proof for its first save.
    if (manual && document.manualExportAttempted) fail('unsaved_changes')
    if (!manual && !document.view.dirty) return
    document.binding = await listBinding(document.view.kind, document.binding)
    if (!document.binding.operations.includes('commit-object') || !document.binding.operations.includes('object-status')) fail('package_unavailable')
    const approved = document.approvedChange
    if (!approved || approved.baseRevision !== document.view.revision) fail('invalid_request')
    const operationId = approved.saveOperationId, revision = document.view.revision
    let freezeAck: NativeOfficeFreezeAck | undefined
    if (manual) {
      freezeAck = await freezeManual(document)
      if (!document.view.dirty) {
        // End this unchanged prepared transaction without a file write. The
        // next input obtains a fresh Core authority against its reread base.
        await cancelChange(document, approved.threadId, manual.changeId)
        return
      }
    }
    if (manual) document.manualExportAttempted = true
    if (diagnostic) diagnostic.phase = 'export'
    const exported = await engine(document, {command:'export', operationId, documentId:document.view.objectId, version:revision,
      ...(manual ? {changeId:manual.changeId,freezeAck} : {})})
    if (freezeAck && exported?.exportedSequence !== freezeAck.sequence) fail('invalid_response')
    if (!exported?.bytes || exported.exportedSequence === undefined) fail('invalid_response')
    const bytes = exported.bytes, digest = hash(bytes)
    const input = nativeOfficeCommitInputSchema.parse({sessionId:document.sessionId, operationId, threadId:approved.threadId, changeId:approved.changeId, baseRevision:revision,
      content:{encoding:'base64', kind:document.view.kind, byteLength:bytes.byteLength, sha256:digest, data:Buffer.from(bytes).toString('base64')}})
    document.pendingSave = {operationId, sequence:exported.exportedSequence, revision, digest}
    document.view.saving = true; publish()
    // Never retry a commit after an uncertain result; subsequent saves query this operation.
    let result: ObjectEditingResponse
    if (diagnostic) diagnostic.phase = 'commit'
    try { result = await invoke(document, 'commit-object', input,diagnostic) } catch { setError(document, 'unknown'); fail('unknown') }
    if (!result.ok && !result.receipt) {
      if (diagnostic) diagnostic.phase = 'ack'
      await engine(document, {command:'ack', operationId:newOperationId(), documentId:document.view.objectId, version:revision,
        exportOperationId:operationId, exportedSequence:exported.exportedSequence, status:'failed'})
      document.pendingSave = undefined; document.view.saving = false
      if (diagnostic) diagnostic.phase = 'commit'
      setError(document, result.code === 'conflict' ? 'conflict' : 'save_failed'); fail(result.code === 'conflict' ? 'conflict' : 'save_failed')
    }
    await settleSave(document, result,diagnostic)
  }

  async function finishRecovery(document: Active) {
    const pending = document.pendingRecovery
    if (!pending) fail('invalid_request')
    if (!pending.committed) {
      const result = await invoke(document, 'object-status', {sessionId:document.sessionId, operationId:pending.operationId})
      await acceptRecoveryReceipt(document, result)
    }
    // A receipt proves that operation, not that another application has left
    // the file unchanged since. Check current Core bytes before every reload.
    const reopened = await invoke(document, 'open-object', {object:{workspace:document.workspace,path:document.requestedPath}})
    if (!reopened.ok || !('document' in reopened)) fail('unknown')
    const current = reopened.document, currentBytes = decodeContent(current.content)
    if (current.objectId !== document.view.objectId || current.sessionId !== document.sessionId || current.path !== document.view.path || current.revision !== hash(currentBytes)) fail('invalid_response')
    if (current.revision !== pending.revision) {
      delete document.pendingRecovery; delete document.pendingSave; delete document.approvedChange
      document.view.saving = false; document.view.canUndo = false; document.view.editing = false
      delete document.view.manual; delete document.manualAuthority; delete document.pendingManualBegin
      delete document.manualFreeze; delete document.manualExportAttempted; delete document.pendingManualCancel
      invalidatePreview(document, 'conflict'); fail('conflict')
    }
    // Core is authoritative. Recreate the readonly view only after its exact
    // recovery receipt; a failed reload remains retryable without another write.
    delete document.view.manual; delete document.manualAuthority; delete document.pendingManualBegin; delete document.nativeWritable
    delete document.manualFreeze; delete document.manualExportAttempted; delete document.pendingManualCancel
    try {
      await document.surface.hide()
      document.ready = false
      await destroySurface(document)
      if (document.playbackStopUnknown && !document.surfaceRetired) fail('unknown')
      delete document.destroyed
      const surface = await options.createSurface(event => { if (document.surface === surface) onEvent(document, event) })
      document.surface = surface; document.failed = false; document.surfaceRetired = false
      document.view = {...document.view, revision:pending.revision, changeSequence:0, acknowledgedSequence:0,
        dirty:false, editing:false, canUndo:false, saving:true, status:'loading', selection:undefined, scope:undefined, proposals:undefined, localReviews:undefined}
      document.openOperationId = newOperationId()
      delete document.view.playback; document.playbackAttempted = false
      document.playbackStopUnknown = false
      await engine(document, {command:'open', documentId:document.view.objectId, version:document.view.revision,
        operationId:document.openOperationId, kind:document.view.kind, bytes:currentBytes})
      document.ready = true
      if (active === document && visible) await surface.attach(document.bounds, document.appearance)
    } catch { setError(document, 'unknown'); fail('unknown') }
    delete document.pendingRecovery; delete document.pendingSave; delete document.approvedChange
    document.selections.clear(); document.decisionOperations.clear()
    document.view.saving = false; document.view.status = 'ready'; delete document.view.error
    await recover(document, document.threadId); publish()
  }
  async function acceptRecoveryReceipt(document: Active, result: ObjectEditingResponse, committing = false) {
    const pending = document.pendingRecovery!
    const conflict = async () => {
      delete document.pendingRecovery; document.view.saving = false
      if (active === document && visible) await document.surface.attach(document.bounds, document.appearance)
      fail('conflict')
    }
    if (!('receipt' in result) || !result.receipt) {
      if (!result.ok && result.code === 'conflict') await conflict()
      if (committing && !result.ok) {
        delete document.pendingRecovery; document.view.saving = false
        if (active === document && visible) await document.surface.attach(document.bounds, document.appearance)
        fail('save_failed')
      }
      fail('unknown')
    }
    const receipt = result.receipt
    if (receipt.operationId !== pending.operationId) fail('invalid_response')
    if (receipt.status === 'conflict') await conflict()
    if (receipt.status !== 'committed') fail('unknown')
    if (receipt.revision !== pending.revision) fail('invalid_response')
    pending.committed = true
  }
  async function recover(document: Active, threadId?: string) {
    if (document.threadId !== threadId) {
      if (document.pendingManualBegin) fail('unknown')
      if (document.manualAuthority) fail('unsaved_changes')
      await flushAnnotation(document)
      if (document.view.dirty || document.pendingSave || document.pendingRecovery) fail('unsaved_changes')
      delete document.view.scope; delete document.view.proposals; delete document.view.localReviews
      delete document.view.annotation; delete document.view.annotationError
      document.threadId = threadId
    }
    if (threadId && !document.view.annotation && document.binding.operations.includes('annotation-read')) await loadAnnotation(document)
    delete document.view.recovery; document.view.canUndo = false
    if (!threadId || !document.binding.operations.includes('object-recovery')) return
    const result = await invokeSelection(document, 'object-recovery', {sessionId:document.sessionId,threadId})
    if (!result.ok || !('recovery' in result)) fail('invalid_response')
    if ([result.recovery.current,result.recovery.pending].some(change => change && change.threadId !== threadId)) fail('invalid_response')
    document.view.recovery = result.recovery
    const current = result.recovery.current
    document.view.canUndo = !!(current?.canUndo || current?.canRetryUndo) && current.revision === document.view.revision && !document.view.dirty
    publish()
  }
  async function loadAnnotation(document: Active) {
    if (!document.threadId) fail('invalid_request')
    const result = await invokeSelection(document, 'annotation-read', {sessionId:document.sessionId,threadId:document.threadId})
    if (!('annotation' in result) || !('draftRevision' in result.annotation) || result.annotation.objectId !== document.view.objectId || result.annotation.threadId !== document.threadId) fail('invalid_response')
    document.view.annotation = result.annotation
    publish()
  }
  async function flushAnnotation(document: Active) {
    if (!document.annotationDesired && !document.annotationAttempt) return
    document.view.annotationSaving = true; document.view.annotationError = false; publish()
    try {
      if (!document.view.annotation) await loadAnnotation(document)
      while (document.annotationDesired || document.annotationAttempt) {
        const saved = document.view.annotation!
        const desired = document.annotationDesired
        if (!document.annotationAttempt && desired?.note === saved.note && desired.sourceRevision === saved.sourceRevision) {
          delete document.annotationDesired
          break
        }
        // Keep the exact failed attempt until Core confirms its CAS. New input
        // can replace the desired note, but cannot change an uncertain request.
        const attempt = document.annotationAttempt ??= {expectedDraftRevision:saved.draftRevision,...desired!}
        const result = await invokeSelection(document, 'annotation-write', {sessionId:document.sessionId,threadId:document.threadId,...attempt})
        if (!('annotation' in result) || !('draftRevision' in result.annotation) || result.annotation.objectId !== document.view.objectId || result.annotation.threadId !== document.threadId ||
          result.annotation.note !== attempt.note || result.annotation.sourceRevision !== attempt.sourceRevision || !result.annotation.draftRevision) fail('invalid_response')
        document.view.annotation = result.annotation
        delete document.annotationAttempt
        if (document.annotationDesired?.note === attempt.note && document.annotationDesired.sourceRevision === attempt.sourceRevision) delete document.annotationDesired
        publish()
      }
      document.view.annotationSaving = false; publish()
    } catch (error) {
      document.view.annotationSaving = false; document.view.annotationError = true; publish()
      throw error
    }
  }
  async function cancelChange(document: Active, threadId: string, changeId: string, reopen = true) {
    if (threadId !== document.threadId || document.pendingRecovery) fail('invalid_request')
    await recover(document, threadId)
    const pending = document.view.recovery?.pending
    const retry = document.pendingManualCancel?.changeId === changeId && document.pendingManualCancel.threadId === threadId
    if ((!pending || pending.changeId !== changeId || !pending.canCancel) && !retry) fail('unknown')
    if (document.manualAuthority) {
      if (document.manualAuthority.changeId !== changeId) fail('invalid_request')
      // Retire the discarded input sink before cancelling its Core authority.
      // Retain the exact cancellation identity across a lost reply; Core's
      // cancelled record is idempotent, while pending=null alone proves nothing.
      document.pendingManualCancel = {changeId,threadId}
      document.ready = false; document.failed = true
      await destroySurface(document)
      if (!document.surfaceRetired) fail('unknown')
    }
    // Cancellation discards only a proven uncommitted working copy. Read the
    // actual Core revision for metadata CAS; no caller supplies original bytes.
    const opened = await invoke(document, 'open-object', {object:{workspace:document.workspace,path:document.requestedPath}})
    if (!opened.ok || !('document' in opened)) fail('unavailable')
    const current = opened.document
    if (current.objectId !== document.view.objectId || current.sessionId !== document.sessionId || current.path !== document.view.path || current.revision !== hash(decodeContent(current.content))) fail('invalid_response')
    let result: NativeOfficeSelectionResponse
    try { result = await invokeSelection(document, 'cancel-change', {sessionId:document.sessionId,threadId,changeId,baseRevision:current.revision}) }
    catch (error) {
      if (document.pendingManualCancel && error instanceof OfficeFailure && error.code === 'unavailable') fail('unknown')
      throw error
    }
    if (!result.ok || !('recovery' in result) || result.recovery.pending?.changeId === changeId) fail('invalid_response')
    delete document.pendingSave; delete document.approvedChange; delete document.manualAuthority; delete document.pendingManualBegin
    delete document.manualFreeze; delete document.manualExportAttempted; delete document.pendingManualCancel
    delete document.view.manual; document.view.editing = false
    document.view.saving = false
    const request = {action:'open' as const,workspace:document.workspace,path:document.requestedPath,bounds:document.bounds,appearance:document.appearance,threadId}
    await close(document, true)
    if (reopen && visible) await open(request, visibilityEpoch)
  }
  async function resumeChange(document: Active, threadId: string, changeId: string) {
    if (threadId !== document.threadId) fail('invalid_request')
    if (document.pendingRecovery) { await finishRecovery(document); return }
    await recover(document, threadId)
    const change = document.view.recovery?.pending
    if (!change || change.changeId !== changeId || !change.canResume || change.baseRevision !== document.view.revision || !change.revision) fail('invalid_request')
    // This is an explicit user continuation, never a status-query side effect.
    // Core retains and validates the exact approved exported candidate.
    document.pendingRecovery = {revision:change.revision,operationId:change.saveOperationId,committed:false}
    document.view.saving = true; publish()
    await document.surface.hide()
    let result: ObjectEditingResponse
    try { result = await invoke(document, 'resume-change', {sessionId:document.sessionId,threadId,changeId,baseRevision:document.view.revision}) } catch { fail('unknown') }
    await acceptRecoveryReceipt(document, result, true)
    delete document.pendingSave
    await finishRecovery(document)
  }
  async function undoChange(document: Active, threadId: string) {
    if (threadId !== document.threadId) fail('invalid_request')
    if (document.pendingRecovery) { await finishRecovery(document); return }
    await recover(document, threadId)
    const change = document.view.recovery?.current
    if (!change || !document.view.canUndo || document.view.dirty || document.pendingSave) fail('invalid_request')
    document.pendingRecovery = {revision:change.baseRevision,operationId:change.undoOperationId,committed:false}
    document.view.saving = true; publish()
    await document.surface.hide()
    let result: ObjectEditingResponse
    try { result = await invoke(document, 'undo-change', {sessionId:document.sessionId,threadId,changeId:change.changeId,baseRevision:document.view.revision}) } catch { fail('unknown') }
    await acceptRecoveryReceipt(document, result, true)
    await finishRecovery(document)
  }

  async function restoreFailedWorkbook(document: Active) {
    // No partially changed worker may export. Recovery loads exact Core bytes;
    // the durable approved change remains pending for explicit cancellation.
    document.failed=true;document.ready=false
    delete document.approvedChange;delete document.pendingSave
    await destroySurface(document)
    if (document.playbackStopUnknown && !document.surfaceRetired) fail('unknown')
    try {
      const opened=await invoke(document,'open-object',{object:{workspace:document.workspace,path:document.requestedPath}})
      if (!opened.ok || !('document' in opened)) fail('unknown')
      const current=opened.document,bytes=decodeContent(current.content)
      if (current.objectId!==document.view.objectId || current.sessionId!==document.sessionId || current.path!==document.view.path || current.revision!==document.view.revision || hash(bytes)!==current.revision) fail('conflict')
      const surface=await options.createSurface(event=>{if(document.surface===surface)onEvent(document,event)})
      document.surface=surface;document.failed=false;document.surfaceRetired=false;delete document.destroyed
      document.view={...document.view,changeSequence:0,acknowledgedSequence:0,dirty:false,editing:false,canUndo:false,saving:false,status:'loading',selection:undefined,scope:undefined,proposals:undefined,localReviews:undefined}
      document.selections.clear();document.decisionOperations.clear();document.openOperationId=newOperationId()
      delete document.view.playback;document.playbackAttempted=false
      document.playbackStopUnknown=false
      await engine(document,{command:'open',documentId:document.view.objectId,version:current.revision,operationId:document.openOperationId,kind:document.view.kind,bytes})
      document.ready=true;document.view.status='ready'
      if(active===document&&visible)await surface.attach(document.bounds,document.appearance)
      await recover(document,document.threadId);publish()
    }catch {document.failed=true;document.ready=false;setError(document,'unknown')}
  }
  async function reference(document: Active, request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, {action:'reference'}>) {
    if (request.editable && document.view.kind === 'xlsx' && !document.view.workbookEditing?.editable) fail('unsupported_selection')
    if (document.threadId !== request.threadId) await recover(document, request.threadId)
    if (document.pendingSave || document.pendingRecovery || document.pendingManualCancel) fail('unknown')
    if (document.view.dirty && !request.saveFirst) fail('unsaved_changes')
    if (document.manualAuthority) {
      // Native input can precede its dirty event. Let the bound worker drain
      // that prefix and validate the real endpoints before any Core reference.
      await engine(document,{command:'validateSelection',operationId:newOperationId(),documentId:document.view.objectId,
        version:document.view.revision,selectionToken:request.selectionToken,expectedChangeSequence:request.expectedChangeSequence})
    }
    if (document.view.dirty) {
      if (!request.saveFirst) fail('unsaved_changes')
      // Validate the chosen native range before freezing. A successful save
      // invalidates every old token; reference only the actual recaptured range.
      if (!document.manualAuthority) await engine(document,{command:'validateSelection',operationId:newOperationId(),documentId:document.view.objectId,
        version:document.view.revision,selectionToken:request.selectionToken,expectedChangeSequence:request.expectedChangeSequence})
      await save(document)
      if (document.view.dirty || document.pendingSave || document.pendingRecovery) fail('unknown')
      await engine(document,{command:'captureSelection',operationId:newOperationId(),documentId:document.view.objectId,version:document.view.revision})
      const recaptured = document.view.selection
      if (!recaptured || recaptured.kind !== 'text' || !recaptured.token || !recaptured.text.trim() || !recaptured.capture?.complete) fail('unsupported_selection')
      request = {...request,revision:document.view.revision,expectedChangeSequence:document.view.changeSequence ?? 0,selectionToken:recaptured.token}
    }
    if (request.editable && document.nativeWritable && !document.view.editing && !document.manualAuthority) {
      await engine(document,{command:'edit',operationId:newOperationId(),documentId:document.view.objectId,version:document.view.revision})
      // Controlled AI preparation must retain the native selection captured
      // after save; its actual current token comes from the correlated reply.
      const current = document.view.selection
      if (!current || current.kind !== 'text' || !current.token || !current.text.trim()) fail('unsupported_selection')
      request = {...request,selectionToken:current.token}
    }
    const selection = document.selections.get(request.selectionToken)
    if (!selection || selection.version !== document.view.revision || selection.changeSequence !== document.view.changeSequence) fail('stale_selection')
    const text = selection.kind === 'shapes' ? selection.shapes.map(s => s.text).join('\n') : selection.kind === 'unavailable' ? '' : selection.kind === 'cells' && !request.editable ? workbookDiscussionText(selection) : selection.text
    const single = selection.kind === 'text' || (selection.kind === 'shapes' && selection.shapes.length === 1) || (selection.kind === 'cells' && selection.cells?.length === 1 && selection.ranges.length === 1 && selection.ranges[0].startRow === selection.ranges[0].endRow && selection.ranges[0].startColumn === selection.ranges[0].endColumn)
    const textCell = selection.kind !== 'cells' || selection.cells?.every(cell =>
      (cell.valueType === 'text' || cell.valueType === 'empty') && !cell.merged)
    const workbook = selection.kind === 'cells' && !(single && selection.cells?.[0]?.valueType === 'text') ? nativeTypedWorkbookSelection(selection) : null
    const presentation = nativeTypedPresentationSelection(selection)
    if (request.editable && (!document.view.editing || (!presentation && !workbook && (!single || !selection.capture?.complete || !text || !textCell)))) fail('unsupported_selection')
    const result = await invokeSelection(document, 'capture-selection', {sessionId:document.sessionId, threadId:request.threadId, selectionToken:request.selectionToken, changeSequence:selection.changeSequence, baseRevision:selection.version, text, editable:request.editable,...(request.editable && workbook ? {workbook} : {}),...(request.editable && presentation ? {presentation} : {})})
    if (!result.ok || !('scope' in result)) fail('invalid_response')
    const parsedScope = nativeOfficeSelectionScopeSchema.safeParse(result.scope)
    if (!parsedScope.success) fail('invalid_response')
    const captured = parsedScope.data
    if (captured.sessionId !== document.sessionId || captured.threadId !== request.threadId ||
      captured.selectionToken !== request.selectionToken || captured.baseRevision !== selection.version ||
      captured.changeSequence !== selection.changeSequence || captured.editable !== request.editable ||
      !request.editable && (captured.workbook || captured.presentation)) fail('invalid_response')
    if (disposed || !document.ready || document.failed || document.view.revision !== selection.version ||
      document.view.changeSequence !== selection.changeSequence) fail('stale_selection')
    const {sessionId: _session, ...scope} = parsedScope.data
    document.revokedScopes.clear(); document.decisionOperations.clear(); document.view.scope = scope; document.view.proposals = []; publish()
  }
  async function proposals(document: Active, scopeId: string) {
    const scope = document.view.scope
    if (!scope || scopeId !== scope.scopeId) fail('stale_selection')
    if (!scope.editable) {
      // Proposal summaries remain readable after revocation. Discussion sends
      // instead need selection-read to recheck scope authority and disk revision.
      const checked = await invokeSelection(document,'selection-read',{sessionId:document.sessionId,scopeId})
      if (!checked.ok || !('scope' in checked)) fail('invalid_response')
      const parsedCurrent = nativeOfficeSelectionScopeSchema.safeParse(checked.scope)
      if (!parsedCurrent.success) fail('invalid_response')
      const current = parsedCurrent.data
      if (current.sessionId !== document.sessionId || current.scopeId !== scopeId || current.threadId !== scope.threadId ||
        current.selectionToken !== scope.selectionToken || current.baseRevision !== document.view.revision ||
        current.changeSequence !== document.view.changeSequence || current.editable || current.workbook || current.presentation) fail('stale_selection')
    }
    const result = await invokeSelection(document, 'proposal-read', {sessionId:document.sessionId,scopeId})
    if (!result.ok || !('proposals' in result)) fail('invalid_response')
    document.view.proposals = result.proposals; document.view.localReviews = result.localReviews; publish()
  }
  async function decideProposal(document: Active, scopeId: string, proposalId: string, accept: boolean) {
    const scope = document.view.scope
    if (!scope || scopeId !== scope.scopeId) fail('stale_selection')
    if (document.appliedProposals.has(proposalId)) return
    if (accept && (!document.view.editing || scope.baseRevision !== document.view.revision || scope.changeSequence !== document.view.changeSequence)) fail('stale_selection')
    if (accept && (document.view.dirty || document.pendingSave || document.pendingRecovery)) fail('unsaved_changes')
    const decisionKey = `${scopeId}:${proposalId}:${accept}`
    let operationId = document.decisionOperations.get(decisionKey)
    if (!operationId) {
      if (document.decisionOperations.size >= 32) fail('capacity')
      operationId = newOperationId(); document.decisionOperations.set(decisionKey, operationId)
    }
    const result = await invokeSelection(document, accept ? 'proposal-accept' : 'proposal-reject', {sessionId:document.sessionId,scopeId,proposalId,operationId,
      ...(accept ? {selectionToken:scope.selectionToken,changeSequence:scope.changeSequence,baseRevision:scope.baseRevision} : {})})
    if (accept) {
      if (!result.ok || !('replacement' in result)) fail('invalid_response')
      const replacement = result.replacement
      if (replacement.operationId !== operationId || replacement.proposalId !== proposalId || replacement.selectionToken !== scope.selectionToken || replacement.changeSequence !== scope.changeSequence || replacement.baseRevision !== scope.baseRevision) fail('invalid_response')
      if (!replacement.workbook && !replacement.presentation && replacement.text.length > 4096) fail('unsupported_selection')
      document.approvedChange = {changeId:replacement.changeId,threadId:scope.threadId,saveOperationId:replacement.saveOperationId,baseRevision:scope.baseRevision}
      document.view.canUndo = false
      if (replacement.presentation) {
        if (scope.workbook || !scope.presentation || JSON.stringify(replacement.presentation.before) !== JSON.stringify(scope.presentation)) fail('invalid_response')
        try { await engine(document,{command:'replacePresentation',operationId:newOperationId(),documentId:document.view.objectId,version:scope.baseRevision,selectionToken:scope.selectionToken,expectedChangeSequence:scope.changeSequence,presentation:replacement.presentation}) }
        catch (error) { if (error instanceof OfficeFailure && error.transient) throw error; await restoreFailedWorkbook(document); fail('save_failed') }
      } else if (replacement.workbook) {
        if (scope.presentation || !scope.workbook || JSON.stringify(replacement.workbook.before) !== JSON.stringify(scope.workbook)) fail('invalid_response')
        try { await engine(document,{command:'replaceCells',operationId:newOperationId(),documentId:document.view.objectId,version:scope.baseRevision,selectionToken:scope.selectionToken,expectedChangeSequence:scope.changeSequence,workbook:replacement.workbook}) }
        catch (error) { recordSaveFailure(document,error,saveDiagnostic('replace')); if (error instanceof OfficeFailure && error.transient) throw error; await restoreFailedWorkbook(document); fail('save_failed') }
      } else {
        if (scope.workbook) fail('invalid_response')
      await engine(document, {command:'replace',operationId:newOperationId(),documentId:document.view.objectId,version:scope.baseRevision,selectionToken:scope.selectionToken,expectedChangeSequence:scope.changeSequence,text:replacement.text,valueType:'text'})
      }
      document.appliedProposals.add(proposalId)
      document.view.appliedProposals = [...document.appliedProposals].slice(-16)
      await save(document)
      return
    }
    await proposals(document,scopeId)
  }

  return {
    getView,
    // UI-only lease; reuse native range validation without issuing a public edit/reference operation.
    async pointerActions(input:NativeOfficeActionMenu): Promise<string|null|NativeOfficeFormatChoice> {
      const parsed = nativeOfficeActionMenuSchema.safeParse(input)
      if (!parsed.success || !parsed.data.pointer) return null
      const target = parsed.data, pointer = parsed.data.pointer, document = active, formatting = target.formatting
      const formattingCurrent = () => {
        if (!formatting) return true
        const manual = document?.view.manual, selected = document?.view.selection
        return !!document && !!document.manualAuthority && document.manualAuthority.version === 2 && document.manualAuthority.origin === 'manual' &&
          document.manualAuthority.baseRevision === document.view.revision && !document.pendingSave && !document.pendingRecovery && !document.pendingManualCancel &&
          !!manual && !manual.frozen && document.view.editing === true && document.manualAuthority.changeId === formatting.changeId &&
          manual.changeId === formatting.changeId && manual.generation === formatting.rangeLease.generation &&
          selected?.kind === 'text' && !!selected.rangeLease && !!selected.formatState &&
          (['generation','sequence','token'] as const).every(key => selected.rangeLease![key] === formatting.rangeLease[key]) &&
          (['fontFamily','fontSize','bold','italic','underline'] as const).every(key => selected.formatState![key] === formatting.formatState[key])
      }
      const current = () => !!document && active === document && visible && !disposed && document.ready && !document.failed && !document.closed && !document.closePending &&
        !document.protectedSnapshot && document.view.kind === 'docx' && document.view.status === 'ready' && !document.view.saving && !isPlaying(document) &&
        document.view.objectId === target.objectId && document.view.revision === target.revision && (document.view.changeSequence ?? 0) === target.expectedChangeSequence &&
        document.view.selection?.token === pointer.selectionToken && document.surface.isPointerCurrent?.(pointer) === true && formattingCurrent()
      const validate = async () => {
        let valid = false
        await serialize(async () => {
          if (!current()) return
          await engine(document!,{command:'validateSelection',operationId:newOperationId(),documentId:target.objectId,version:target.revision,
            selectionToken:pointer.selectionToken,expectedChangeSequence:target.expectedChangeSequence})
          valid = current()
        })
        return valid
      }
      try {
        if (!await validate()) return null
        const action = await document!.surface.presentPointerActions?.(target)
        if (!action) return null
        if (typeof action === 'string') {
          if (!target.actions.some(a => a.id === action && a.enabled) || !await validate()) return null
          return action
        }
        const choice = nativeOfficeFormatChoiceSchema.safeParse(action)
        if (!choice.success || !formatting || choice.data.changeId !== formatting.changeId ||
          !(['generation','sequence','token'] as const).every(key => choice.data.rangeLease[key] === formatting.rangeLease[key]) || !await validate()) return null
        return choice.data
      } finally { document?.surface.dismissPointerActions?.(pointer) }
    },
    openPresent(request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, {action:'openPresent'}>, snapshot: PresentOfficeSnapshot) {
      const parsed = nativeOfficeRequestSchema.safeParse(request)
      if (!parsed.success || parsed.data.action !== 'openPresent' || JSON.stringify(parsed.data.protectedPresent) !== JSON.stringify(snapshot.selector)) return Promise.resolve(response('invalid_request'))
      const epoch = ++visibilityEpoch; visible = true
      return serialize(() => openProtected(parsed.data as Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, {action:'openPresent'}>,snapshot,epoch)).then(result =>
        result.view?.protectedPresent && JSON.stringify(result.view.protectedPresent) === JSON.stringify(snapshot.selector) ? result : {...result,view:null})
    },
    restoreVisibility: async () => { visible = true; if (active?.ready && !active.pendingRecovery) { const document = active; await guardPresent(document); await document.surface.attach(document.bounds, document.appearance); await guardPresent(document) } },
    getViews: () => [...documents.values()].map(d => structuredClone(d.view)),
    freezeAnnotationInput,
    hasPendingAnnotations: () => [...documents.values()].some(d => !!d.annotationDesired || !!d.annotationAttempt),
    flushAnnotations: () => serialize(async () => { for (const document of documents.values()) await flushAnnotation(document) }),
    saveAll: () => serialize(async () => { for (const document of documents.values()) if (document.manualAuthority || document.view.dirty || document.pendingSave || document.pendingRecovery) await save(document) }),
    /** Owner teardown releases read-only resources. */
    destroy(): Promise<void> {
      if (shutdown) return shutdown
      disposed = true
      const closing = [...documents.values()]
      // Destroy first to reject any in-flight preview request; serialize Core
      // cleanup behind the pending open/capture operation.
      shutdown = Promise.resolve().then(() => Promise.all(closing.map(destroySurface))).then(async () => {
        await serialize(async () => {
          for (const document of closing) {
          try { if (!document.protectedSnapshot) await invoke(document, 'close-object', { sessionId: document.sessionId }) } catch { /* Teardown never fabricates a successful Core close. */ }
          finally {
            await destroySurface(document)
            if (active === document) active = undefined
            if (documents.get(document.view.objectId) === document) documents.delete(document.view.objectId)
            publish()
          }
          }
        })
      })
      return shutdown
    },
    request(payload: unknown): Promise<NativeOfficeResponse> {
      if (disposed) return Promise.resolve(response('unavailable'))
      const parsed = nativeOfficeRequestSchema.safeParse(payload)
      if (!parsed.success) return Promise.resolve(response('invalid_request'))
      if (parsed.data.action === 'openPresent') return Promise.resolve(response('invalid_request'))
      if (parsed.data.action === 'create') return createObject(parsed.data)

      if (parsed.data.action === 'annotationSave') {
        const request = parsed.data, document = documents.get(request.objectId)
        if (annotationInputFrozen || !document || document.protectedSnapshot || document.closed || document.closePending || document.threadId !== request.threadId) return Promise.resolve(response('unavailable'))
        // Record synchronously before queueing so a close event cannot miss the
        // most recent keystroke while another native operation is in flight.
        document.annotationDesired = {note:request.note,sourceRevision:request.sourceRevision}
        document.view.annotationSaving = true; document.view.annotationError = false; publish()
        return serialize(async () => { await flushAnnotation(document) })
      }
      const changesVisibility = ['hide', 'open'].includes(parsed.data.action)
      const epoch = changesVisibility ? ++visibilityEpoch : visibilityEpoch
      if (parsed.data.action === 'hide') {
        const hidden = active
        const hiddenOwner = hidden ? {surface:hidden.surface,objectId:hidden.view.objectId,revision:hidden.view.revision,changeSequence:hidden.view.changeSequence ?? 0} : undefined
        visible = false
        void Promise.resolve(hidden?.surface.hide()).catch(() => undefined)
        // Hide remains immediate while an earlier request is in flight. Its
        // cleanup is queued with this exact owner ahead of later open/control.
        if (hidden) void serialize(async () => { await stopPlayback(hidden, hiddenOwner); publish() })
        return Promise.resolve(response())
      }
      if (parsed.data.action === 'open' || parsed.data.action === 'bounds') visible = true
      return serialize(async () => {
        if (disposed) fail('unavailable')
        const request = parsed.data
        if (request.action === 'open') { await open(request, epoch); if (!visible || epoch !== visibilityEpoch) await active?.surface.hide(); return }
        if (request.action === 'close') { const target = request.objectId ? documents.get(request.objectId) : active; if (target) await close(target, request.discard); return }
        if (request.action === 'status') {
          if (active) {
            await guardPresent(active)
            try {
              active.binding = await listBinding(active.view.kind, active.binding)
              if (active.ready && !active.pendingSave && !active.pendingRecovery) { active.view.status = 'ready'; delete active.view.error; publish() }
            } catch (error) {
              setError(active, error instanceof OfficeFailure ? error.code : 'package_unavailable')
              throw error
            }
          }
          return
        }
        if (request.action === 'hide') { await active?.surface.hide(); return }
        // A lost cancellation reply retains only its recovery owner. The
        // retired native sink cannot admit input or references while Core's
        // idempotent cancellation is still awaiting confirmation.
        if (active?.pendingManualCancel && ['beginManualEdit','editWorkbookCell','format','reference','captureSelection','annotate'].includes(request.action)) {
          if (request.action === 'beginManualEdit') logNativeOfficeManualBeginFailure('unknown',manualBeginFailureState(active))
          fail('unknown')
        }
        const document = ['save','saveStatus','proposals','rejectProposal','annotationRetry','cancelChange','discardManualEdit'].includes(request.action) && 'objectId' in request ? documents.get(request.objectId) : requireActive()
        if (!document || document.closePending) fail('unavailable')
        if (document.playbackStopUnknown && !document.surfaceRetired) fail('unknown')
        if (document.protectedSnapshot && !['viewControl','bounds'].includes(request.action)) fail('invalid_request')
        await guardPresent(document)
        if (request.action === 'annotationRetry') {
          if (document.threadId !== request.threadId) fail('invalid_request')
          await flushAnnotation(document); return
        }
        if (request.action === 'viewControl') {
          // Reject stale controls before the ordinary error path can mark a new tab.
          const owns = () => active === document && document.ready && visible && epoch === visibilityEpoch &&
            request.objectId === document.view.objectId && request.revision === document.view.revision &&
            request.expectedChangeSequence === (document.view.changeSequence ?? 0)
          if (!owns()) fail('stale_selection')
          if (document.pendingSave || document.pendingRecovery) fail('unavailable')
          const playbackControl = request.control.startsWith('play-')
          if (playbackControl && document.view.kind !== 'pptx') fail('invalid_request')
          if (playbackControl && request.control !== 'play-read' && request.playbackGeneration !== (document.view.playback?.generation ?? 0)) fail('stale_selection')
          if (['play-start','play-current'].includes(request.control) && document.view.dirty) fail('unsaved_changes')
          if (!playbackControl && (isPlaying(document) || document.view.playback?.phase === 'error')) fail('unavailable')
          try {
            document.binding = await listBinding(document.view.kind, document.binding)
            if (!owns()) fail('stale_selection')
            if (playbackControl && request.control !== 'play-read' && request.playbackGeneration !== (document.view.playback?.generation ?? 0)) fail('stale_selection')
            document.viewControlEpoch = epoch
            if (request.control === 'play-start' || request.control === 'play-current') document.playbackAttempted = true
            await engine(document, {command:'local-view', action:request.control, ...(request.control === 'set-zoom' ? {zoom:request.zoom} : {}),
              ...(request.playbackGeneration !== undefined ? {playbackGeneration:request.playbackGeneration} : {}),
              operationId:newOperationId(), documentId:request.objectId, version:request.revision}, owns)
            if (!owns()) fail('stale_selection')
            if (playbackControl && document.view.error?.startsWith('presentation_')) delete document.view.error
            publish()
          } catch (error) {
            if (owns()) {
              if (!playbackControl) delete document.view.previewView
              const code = error instanceof OfficeFailure ? error.code : 'unavailable'
              if (playbackControl && code.startsWith('presentation_')) document.view.error = code
              if (code === 'invalid_response' || code === 'engine_unavailable') invalidatePreview(document, code)
              else publish()
            }
            throw error
          } finally { if (document.viewControlEpoch === epoch) delete document.viewControlEpoch }
          return
        }
        if (request.action === 'bounds') { if (visible && epoch === visibilityEpoch) { document.bounds = request.bounds; document.appearance = request.appearance; if (!document.pendingRecovery) await document.surface.attach(request.bounds, request.appearance); await guardPresent(document) } return }
        if (isPlaying(document) || document.view.playback?.phase === 'error') fail('unavailable')
        try {
          document.binding = await listBinding(document.view.kind, document.binding)
          if (request.action === 'proposals') { await proposals(document,request.scopeId); return }
          if (request.action === 'rejectProposal') { await decideProposal(document,request.scopeId,request.proposalId,false); return }
          if (request.action === 'saveStatus') {
            if (document.pendingManualCancel) { await recover(document,document.threadId); fail('unknown') }
            if (!document.pendingSave && !document.pendingRecovery) { await recover(document, document.threadId); return }
            await save(document); return
          }
          if (document.pendingRecovery && request.action !== 'save') {
            if (request.action === 'beginManualEdit') logNativeOfficeManualBeginFailure('unknown',manualBeginFailureState(document))
            fail('unknown')
          }
          if ('objectId' in request && (request.objectId !== document.view.objectId || !('revision' in request) || request.revision !== document.view.revision || request.expectedChangeSequence !== (document.view.changeSequence ?? 0))) fail('stale_selection')
          if (request.action === 'beginManualEdit') { await beginManualEdit(document,request.threadId); return }
          if (request.action === 'editWorkbookCell') { await editWorkbookCell(document,request); return }
          if (request.action === 'format') { await formatManual(document,request); return }
          if (request.action === 'discardManualEdit') {
            if (document.view.kind !== 'docx' || request.threadId !== document.threadId) fail('invalid_request')
            await recover(document,request.threadId)
            const pending = document.view.recovery?.pending
            const retry = document.pendingManualCancel?.changeId === request.changeId && document.pendingManualCancel.threadId === request.threadId
            if ((!pending || !('version' in pending) || pending.version !== 2 || pending.origin !== 'manual' || pending.changeId !== request.changeId) && !retry) fail('invalid_request')
            await cancelChange(document,request.threadId,request.changeId); return
          }
          if (request.action === 'reference') { await reference(document,request); return }
          if (request.action === 'acceptProposal') { await decideProposal(document,request.scopeId,request.proposalId,true); return }
          if (request.action === 'resumeChange') { await resumeChange(document, request.threadId, request.changeId); return }
          if (request.action === 'cancelChange') { await cancelChange(document, request.threadId, request.changeId); return }
          if (request.action === 'undoChange') { await undoChange(document, request.threadId); return }
          if (request.action === 'save') { await save(document); return }
          if (request.action === 'annotate' && document.manualAuthority) fail('invalid_request')
          if (request.action === 'annotate' && (!document.binding.operations.includes('commit-object') || !document.binding.operations.includes('object-status'))) fail('package_unavailable')
          if (!['captureSelection','annotate'].includes(request.action)) fail('invalid_request')
          await engine(document, { command: request.action === 'annotate' ? 'edit' : 'captureSelection', operationId: newOperationId(), documentId: document.view.objectId, version: document.view.revision })
          document.view.status = 'ready'
          delete document.view.error
          publish()
        } catch (error) {
          if (error instanceof OfficeFailure && error.transient) { publish(); throw error }
          const code = error instanceof OfficeFailure ? error.code : 'engine_unavailable'
          const selectionRequest = ['reference','proposals','acceptProposal','rejectProposal'].includes(request.action)
          if ((code === 'invalid_response' && !selectionRequest) || code === 'engine_unavailable') invalidatePreview(document, code)
          else setError(document, code)
          throw error
        }
      })
    }
  }
}
