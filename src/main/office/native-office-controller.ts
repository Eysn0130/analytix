import { isOfficeResult, type OfficeRequestInput } from './office-protocol'
import { nativeOfficeCommitInputSchema, nativeOfficeSelectionResponseSchema, nativeOfficeSelectionScopeSchema, type NativeOfficeSelectionResponse } from '../../../packages/runtime/src/contracts/native-office-editing'
import { createHash, randomUUID } from 'node:crypto'
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
  nativeOfficeRequestSchema, nativeOfficeSelectionSchema, type NativeOfficeBounds, type NativeOfficeAppearance,
  type NativeOfficeError, type NativeOfficeKind, type NativeOfficeResponse,
  type NativeOfficeSelection, type NativeOfficeView
} from '../../shared/native-office'

export const MAX_NATIVE_OFFICE_BYTES = 16 * 1024 * 1024
const MAX_BASE64_LENGTH = Math.ceil(MAX_NATIVE_OFFICE_BYTES / 3) * 4
const packageIds = { docx: 'analytix-documents', xlsx: 'analytix-spreadsheets', pptx: 'analytix-presentations' } as const
const operations = ['open-object', 'close-object'] as const
type HostOperation = typeof operations[number] | 'commit-object' | 'object-status' | 'capture-selection' | 'selection-read' | 'proposal-read' | 'proposal-accept' | 'proposal-reject' | 'selection-revoke'
const token = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/)
const digest = z.string().regex(/^[a-f0-9]{64}$/)
const sequence = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
const engineStateSchema = z.object({
  documentId: digest, version: digest, kind: z.enum(['docx', 'xlsx', 'pptx']),
  changeSequence: sequence, acknowledgedSequence: sequence, dirty: z.boolean()
}).strict().refine(s => s.acknowledgedSequence <= s.changeSequence && s.dirty === (s.changeSequence !== s.acknowledgedSequence))
type EngineState = z.infer<typeof engineStateSchema>
const eventBase = { documentId: digest, version: digest, operationId: token }
const engineEventSchema = z.discriminatedUnion('type', [
  z.object({ ...eventBase, type: z.literal('changed'), state: engineStateSchema }).strict(),
  z.object({ ...eventBase, type: z.literal('selection'), selection: nativeOfficeSelectionSchema }).strict(),
  z.object({ ...eventBase, type: z.literal('save-requested') }).strict()
])

export type NativeOfficeEngineRequest = OfficeRequestInput
/** The isolated surface owns its channel and transport correlation. */
export interface NativeOfficeSurface {
  attach(bounds: NativeOfficeBounds, appearance?: NativeOfficeAppearance): void | Promise<void>
  hide(): void | Promise<void>
  request(envelope: NativeOfficeEngineRequest): Promise<unknown>
  destroy(): void | Promise<void>
}
export interface NativeOfficeControllerOptions {
  packageHost(request: PluginPackageHostRequest): Promise<PluginPackageHostResponse>
  createSurface(onEvent: (event: unknown) => void): NativeOfficeSurface | Promise<NativeOfficeSurface>
  onChange(view: NativeOfficeView | null): void
  newOperationId?: () => string
}

type Active = {
  view: NativeOfficeView; workspace: string; requestedPath: string; sessionId: string
  bounds: NativeOfficeBounds; appearance?: NativeOfficeAppearance; binding: PluginPackageView; surface: NativeOfficeSurface; openOperationId: string
  ready: boolean; failed: boolean; destroyed?: Promise<void>
  selections: Map<string, NativeOfficeSelection>
  appliedProposals: Set<string>
  revokedScopes: Set<string>
  decisionOperations: Map<string, string>
  pendingSave?: { operationId: string; sequence: number; revision: string; digest: string }
  // One bounded, Main-only checkpoint for the last accepted AI change.
  beforeChange?: { bytes: Uint8Array; revision: string }
  undo?: { bytes: Uint8Array; revision: string }
  pendingUndo?: { bytes: Uint8Array; operationId: string; revision: string; committed: boolean }
}
type PreviewEngineRequest = NativeOfficeEngineRequest
class OfficeFailure extends Error {
  constructor(readonly code: NativeOfficeError) { super(code) }
}
function fail(code: NativeOfficeError): never { throw new OfficeFailure(code) }
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

/** Core-bound native sessions. At most two retained surfaces, only one visible. */
export function createNativeOfficeController(options: NativeOfficeControllerOptions) {
  let active: Active | undefined
  const documents = new Map<string, Active>()
  let visibilityEpoch = 0
  let visible = true
  let tail = Promise.resolve()
  let disposed = false
  let shutdown: Promise<void> | undefined
  const newOperationId = () => {
    const id = (options.newOperationId ?? randomUUID)()
    if (!token.safeParse(id).success) fail('unavailable')
    return id
  }
  const getView = (): NativeOfficeView | null => active ? structuredClone(active.view) : null
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

  async function invokeOutput(document: { binding: PluginPackageView; view: Pick<NativeOfficeView, 'kind'> }, operation: HostOperation, input: Record<string, unknown>): Promise<unknown> {
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
    if (!outer.success) fail('invalid_response')
    if (!outer.data.ok) fail('package_unavailable')
    if (!('output' in outer.data)) fail('invalid_response')
    return outer.data.output
  }
  async function invoke(document: { binding: PluginPackageView; view: Pick<NativeOfficeView, 'kind'> }, operation: HostOperation, input: Record<string, unknown>): Promise<ObjectEditingResponse> {
    const inner = objectEditingResponseSchema.safeParse(await invokeOutput(document, operation, input))
    if (!inner.success) fail('invalid_response')
    return inner.data
  }

  async function invokeSelection(document: Active, operation: HostOperation, input: Record<string, unknown>): Promise<NativeOfficeSelectionResponse> {
    const result = nativeOfficeSelectionResponseSchema.safeParse(await invokeOutput(document, operation, input))
    if (!result.success) fail('invalid_response')
    if (!result.data.ok) fail(result.data.code === 'conflict' ? 'conflict' : result.data.code === 'scope_invalid' ? 'stale_selection' : 'unavailable')
    return result.data
  }

  const destroySurface = (document: Active): Promise<void> => {
    document.destroyed ??= Promise.resolve().then(() => document.surface.destroy()).catch(() => undefined)
    return document.destroyed
  }
  const invalidatePreview = (document: Active, code: NativeOfficeError) => {
    document.ready = false
    document.failed = true
    delete document.view.selection
    setError(document, code)
    if (!document.view.dirty) void destroySurface(document)
  }
  function acceptState(document: Active, state: EngineState, expectedVersion = document.view.revision) {
    if (state.documentId !== document.view.objectId || state.version !== expectedVersion || state.kind !== document.view.kind ||
        state.changeSequence < (document.view.changeSequence ?? 0) || (!document.view.editing && (state.dirty || state.changeSequence !== 0 || state.acknowledgedSequence !== 0))) fail('invalid_response')
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
  async function engine(document: Active, request: PreviewEngineRequest) {
    if (disposed || document.failed) fail('engine_unavailable')
    let raw: unknown
    try { raw = await document.surface.request(request) } catch (error) {
      const code = error instanceof Error ? error.message : ''
      if (code === 'stale-selection') fail('stale_selection')
      if (['unsupported-selection', 'invalid-control-value', 'unsupported-command', 'command-unavailable', 'single-cell-required'].includes(code)) fail('unsupported_selection')
      if (code === 'export-too-large') fail('save_failed')
      if (code === 'unsaved-changes') fail('unsaved_changes')
      fail('engine_unavailable')
    }
    if (disposed) fail('engine_unavailable')
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
    if (request.command === 'edit') document.view.editing = true
    acceptState(document, value.state, request.command === 'ack' && request.status === 'committed' ? request.persistedVersion : document.view.revision)
    if (request.command === 'captureSelection' && !value.selection) fail('invalid_response')
    if (value.selection) { const selection = nativeOfficeSelectionSchema.safeParse(value.selection); if (!selection.success) fail('invalid_response'); acceptSelection(document, selection.data) }
    return value
  }

  function onEvent(document: Active, raw: unknown) {
    if (disposed || !documents.has(document.view.objectId) || document.failed) return
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
    if (event.type === 'save-requested') { if (document.view.editing) void serialize(() => save(document)); return }
    if (event.type !== 'selection') return
    try { acceptSelection(document, event.selection); publish() }
    catch { invalidatePreview(document, 'invalid_response') }
  }

  async function open(request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, { action: 'open' }>, epoch: number) {
    if (!isAbsolute(request.workspace)) fail('invalid_request')
    const kind = extname(request.path).slice(1).toLowerCase() as NativeOfficeKind
    if (!Object.hasOwn(packageIds, kind)) fail('invalid_request')
    const retained = [...documents.values()].find(d => d.workspace === request.workspace && resolve(d.workspace, d.requestedPath) === resolve(request.workspace, request.path) && d.ready)
    if (retained) {
      if (active !== retained) await active?.surface.hide()
      active = retained
    }
    if (active) {
      if (active.ready && active.workspace === request.workspace && resolve(active.workspace, active.requestedPath) === resolve(request.workspace, request.path)) {
        active.binding = await listBinding(active.view.kind, active.binding)
        active.bounds = request.bounds; active.appearance = request.appearance
        if (active.pendingUndo) fail('unknown')
        if (visible && epoch === visibilityEpoch) await active.surface.attach(request.bounds, request.appearance)
        active.view.status = 'ready'
        delete active.view.error
        publish()
        return
      }
      await active.surface.hide()
      if (!active.ready && !active.view.dirty) await close(active, true)
    }
    if (documents.size >= 2) {
      const clean = [...documents.values()].find(d => !d.view.dirty && !d.pendingSave && !d.pendingUndo && d !== retained)
      if (!clean) fail('capacity')
      await close(clean)
    }
    const binding = await listBinding(kind)
    let surface: NativeOfficeSurface | undefined
    let document: Active | undefined
    let cleanup: { binding: PluginPackageView; view: Pick<NativeOfficeView, 'kind'>; sessionId: string } | undefined
    try {
      // A temporary Main-owned holder lets every Core invoke use the same
      // current binding validation without exposing session state to Renderer.
      const holder = { binding, view: { kind } }
      const result = await invoke(holder, 'open-object', { object: { workspace: request.workspace, path: request.path } })
      if (!result.ok || !('document' in result)) fail('unavailable')
      const opened = result.document
      cleanup = { ...holder, sessionId: opened.sessionId }
      if (epoch !== visibilityEpoch || !visible) fail('unavailable')
      const bytes = decodeContent(opened.content)
      if (!isAbsolute(opened.path) || resolve(opened.path) !== resolve(request.workspace, request.path) || opened.revision !== hash(bytes) || extname(opened.path).toLowerCase() !== '.' + kind) fail('invalid_response')
      if (disposed) fail('unavailable')
      surface = await options.createSurface(event => { if (document && document.surface === surface) onEvent(document, event) })
      if (disposed || epoch !== visibilityEpoch || !visible) fail('unavailable')
      document = { view: { objectId: opened.objectId, path: opened.path, kind, revision: opened.revision, dirty: false, status: 'loading' },
        workspace: request.workspace, requestedPath: request.path, sessionId: opened.sessionId,
        bounds:request.bounds, appearance:request.appearance, binding: holder.binding, selections:new Map(), appliedProposals:new Set(), revokedScopes:new Set(), decisionOperations:new Map(), surface, openOperationId: newOperationId(), ready: false, failed: false }
      documents.set(document.view.objectId, document)
      active = document
      publish()
      await surface.attach(request.bounds, request.appearance)
      await engine(document, { command: 'open', documentId: opened.objectId, version: opened.revision, operationId: document.openOperationId, kind, bytes })
      document.ready = true
      document.view.status = 'ready'
      publish()
    } catch (error) {
      if (document) await destroySurface(document)
      else { try { await surface?.destroy() } catch { /* Failed preview owns no usable surface. */ } }
      if (cleanup) {
        try { await invoke(cleanup, 'close-object', { sessionId: cleanup.sessionId }) } catch { /* Best-effort cleanup of a failed open only. */ }
      }
      if (document) documents.delete(document.view.objectId)
      if (active === document) active = undefined
      publish()
      throw error
    }
  }

  async function close(document = active, discard = false) {
    if (!document) return
    if (document.pendingSave || document.pendingUndo) fail('unknown')
    if (document.view.dirty && !discard) fail('unsaved_changes')
    if (document.ready && !document.failed) {
      try { await engine(document, { command: 'close', documentId: document.view.objectId, version: document.view.revision,
        operationId: newOperationId(), expectedChangeSequence: document.view.changeSequence ?? 0, discard }) } catch (error) { if (document.view.dirty && !discard) throw error }
    }
    document.ready = false
    try {
      const result = await invoke(document, 'close-object', { sessionId: document.sessionId })
      if (!result.ok || !('closed' in result)) fail('unavailable')
    } finally {
      await destroySurface(document)
      documents.delete(document.view.objectId)
      if (active === document) active = undefined
      publish()
    }
  }
  async function settleSave(document: Active, result: ObjectEditingResponse) {
    const pending = document.pendingSave
    if (!pending || !('receipt' in result) || !result.receipt) fail('unknown')
    const receipt = result.receipt
    if (receipt.operationId !== pending.operationId) fail('invalid_response')
    if (receipt.status === 'unknown' || receipt.status === 'pending') fail('unknown')
    if (receipt.status === 'committed' && receipt.revision !== pending.digest) fail('invalid_response')
    if (receipt.status === 'committed') {
      delete document.view.scope; delete document.view.proposals; document.selections.clear()
    }
    await engine(document, {command:'ack', documentId:document.view.objectId, version:pending.revision,
      operationId:newOperationId(), exportOperationId:pending.operationId, exportedSequence:pending.sequence,
      status:receipt.status === 'committed' ? 'committed' : 'conflict',
      ...(receipt.status === 'committed' ? {persistedVersion:receipt.revision} : {})})
    document.pendingSave = undefined; document.view.saving = false
    if (receipt.status === 'conflict') { setError(document, 'conflict'); fail('conflict') }
    if (document.beforeChange && document.beforeChange.revision === pending.revision && !document.view.dirty) {
      document.undo = {bytes:document.beforeChange.bytes, revision:receipt.revision!}
      delete document.beforeChange
      document.view.canUndo = true
    }
    document.view.status = 'ready'; delete document.view.error; publish()
  }
  async function save(document: Active) {
    if (document.pendingUndo) { await finishUndo(document); return }
    if (!document.view.editing) fail('invalid_request')
    if (document.pendingSave) {
      await settleSave(document, await invoke(document, 'object-status', {sessionId:document.sessionId, operationId:document.pendingSave.operationId}))
      return
    }
    if (!document.view.dirty) return
    document.binding = await listBinding(document.view.kind, document.binding)
    if (!document.binding.operations.includes('commit-object') || !document.binding.operations.includes('object-status')) fail('package_unavailable')
    const operationId = newOperationId(), revision = document.view.revision
    const exported = await engine(document, {command:'export', operationId, documentId:document.view.objectId, version:revision})
    if (!exported?.bytes || exported.exportedSequence === undefined) fail('invalid_response')
    const bytes = exported.bytes, digest = hash(bytes)
    const input = nativeOfficeCommitInputSchema.parse({sessionId:document.sessionId, operationId, baseRevision:revision,
      content:{encoding:'base64', kind:document.view.kind, byteLength:bytes.byteLength, sha256:digest, data:Buffer.from(bytes).toString('base64')}})
    document.pendingSave = {operationId, sequence:exported.exportedSequence, revision, digest}
    document.view.saving = true; publish()
    // Never retry a commit after an uncertain result; subsequent saves query this operation.
    let result: ObjectEditingResponse
    try { result = await invoke(document, 'commit-object', input) } catch { setError(document, 'unknown'); fail('unknown') }
    if (!result.ok && !result.receipt) {
      await engine(document, {command:'ack', operationId:newOperationId(), documentId:document.view.objectId, version:revision,
        exportOperationId:operationId, exportedSequence:exported.exportedSequence, status:'failed'})
      document.pendingSave = undefined; document.view.saving = false
      setError(document, result.code === 'conflict' ? 'conflict' : 'save_failed'); fail(result.code === 'conflict' ? 'conflict' : 'save_failed')
    }
    await settleSave(document, result)
  }

  async function finishUndo(document: Active) {
    const pending = document.pendingUndo
    if (!pending) fail('invalid_request')
    if (!pending.committed) {
      const result = await invoke(document, 'object-status', {sessionId:document.sessionId, operationId:pending.operationId})
      await acceptUndoReceipt(document, result)
    }
    // A receipt proves that operation, not that another application has left
    // the file unchanged since. Check current Core bytes before every reload.
    const reopened = await invoke(document, 'open-object', {object:{workspace:document.workspace,path:document.requestedPath}})
    if (!reopened.ok || !('document' in reopened)) fail('unknown')
    const current = reopened.document, currentBytes = decodeContent(current.content)
    if (current.objectId !== document.view.objectId || current.sessionId !== document.sessionId || current.path !== document.view.path || current.revision !== hash(currentBytes)) fail('invalid_response')
    if (current.revision !== hash(pending.bytes)) {
      delete document.pendingUndo; delete document.undo; delete document.beforeChange
      document.view.saving = false; document.view.canUndo = false; document.view.editing = false
      invalidatePreview(document, 'conflict'); fail('conflict')
    }
    // Core is authoritative. Recreate the readonly view only after its exact
    // rollback receipt; a failed reload remains retryable without another write.
    try {
      await document.surface.hide()
      document.ready = false
      await destroySurface(document)
      delete document.destroyed
      const surface = await options.createSurface(event => { if (document.surface === surface) onEvent(document, event) })
      document.surface = surface; document.failed = false
      document.view = {...document.view, revision:hash(pending.bytes), changeSequence:0, acknowledgedSequence:0,
        dirty:false, editing:false, canUndo:false, saving:true, status:'loading', selection:undefined, scope:undefined, proposals:undefined}
      document.openOperationId = newOperationId()
      await engine(document, {command:'open', documentId:document.view.objectId, version:document.view.revision,
        operationId:document.openOperationId, kind:document.view.kind, bytes:pending.bytes})
      document.ready = true
      if (active === document && visible) await surface.attach(document.bounds, document.appearance)
    } catch { setError(document, 'unknown'); fail('unknown') }
    delete document.pendingUndo; delete document.undo; delete document.beforeChange
    document.selections.clear(); document.decisionOperations.clear()
    document.view.saving = false; document.view.status = 'ready'; delete document.view.error; publish()
  }
  async function acceptUndoReceipt(document: Active, result: ObjectEditingResponse, committing = false) {
    const pending = document.pendingUndo!
    const conflict = async () => {
      delete document.pendingUndo; document.view.saving = false
      if (active === document && visible) await document.surface.attach(document.bounds, document.appearance)
      fail('conflict')
    }
    if (!('receipt' in result) || !result.receipt) {
      if (!result.ok && result.code === 'conflict') await conflict()
      if (committing && !result.ok) {
        delete document.pendingUndo; document.view.saving = false
        if (active === document && visible) await document.surface.attach(document.bounds, document.appearance)
        fail('save_failed')
      }
      fail('unknown')
    }
    const receipt = result.receipt
    if (receipt.operationId !== pending.operationId) fail('invalid_response')
    if (receipt.status === 'conflict') await conflict()
    if (receipt.status !== 'committed') fail('unknown')
    if (receipt.revision !== hash(pending.bytes)) fail('invalid_response')
    pending.committed = true
  }
  async function undoChange(document: Active) {
    if (document.pendingUndo) { await finishUndo(document); return }
    if (!document.undo || !document.view.canUndo || document.view.dirty || document.pendingSave) fail('invalid_request')
    if (document.undo.revision !== document.view.revision) fail('conflict')
    const {bytes, revision} = document.undo, operationId = newOperationId()
    document.pendingUndo = {bytes, revision, operationId, committed:false}
    document.view.saving = true; publish()
    await document.surface.hide()
    const input = nativeOfficeCommitInputSchema.parse({sessionId:document.sessionId, operationId, baseRevision:revision,
      content:{encoding:'base64',kind:document.view.kind,byteLength:bytes.byteLength,sha256:hash(bytes),data:Buffer.from(bytes).toString('base64')}})
    let result: ObjectEditingResponse
    try { result = await invoke(document, 'commit-object', input) } catch { fail('unknown') }
    await acceptUndoReceipt(document, result, true)
    await finishUndo(document)
  }

  async function reference(document: Active, request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, {action:'reference'}>) {
    const selection = document.selections.get(request.selectionToken)
    if (!selection || selection.version !== document.view.revision || selection.changeSequence !== document.view.changeSequence) fail('stale_selection')
    const text = selection.kind === 'shapes' ? selection.shapes.map(s => s.text).join('\n') : selection.kind === 'unavailable' ? '' : selection.text
    const single = selection.kind === 'text' || (selection.kind === 'shapes' && selection.shapes.length === 1) || (selection.kind === 'cells' && selection.cells?.length === 1 && selection.ranges.length === 1 && selection.ranges[0].startRow === selection.ranges[0].endRow && selection.ranges[0].startColumn === selection.ranges[0].endColumn)
    if (request.editable && (!document.view.editing || !single || !selection.capture?.complete || !text)) fail('unsupported_selection')
    const result = await invokeSelection(document, 'capture-selection', {sessionId:document.sessionId, threadId:request.threadId, selectionToken:request.selectionToken, changeSequence:selection.changeSequence, baseRevision:selection.version, text, editable:request.editable})
    if (!result.ok || !('scope' in result)) fail('invalid_response')
    const parsedScope = nativeOfficeSelectionScopeSchema.safeParse(result.scope)
    if (!parsedScope.success) fail('invalid_response')
    const {sessionId: _session, ...scope} = parsedScope.data
    document.revokedScopes.clear(); document.decisionOperations.clear(); document.view.scope = scope; document.view.proposals = []; publish()
  }
  async function proposals(document: Active, scopeId: string) {
    if (scopeId !== document.view.scope?.scopeId) fail('stale_selection')
    const result = await invokeSelection(document, 'proposal-read', {sessionId:document.sessionId,scopeId})
    if (!result.ok || !('proposals' in result)) fail('invalid_response')
    document.view.proposals = result.proposals; publish()
  }
  async function decideProposal(document: Active, scopeId: string, proposalId: string, accept: boolean) {
    const scope = document.view.scope
    if (!scope || scopeId !== scope.scopeId) fail('stale_selection')
    if (document.appliedProposals.has(proposalId)) return
    if (accept && (!document.view.editing || scope.baseRevision !== document.view.revision || scope.changeSequence !== document.view.changeSequence)) fail('stale_selection')
    if (accept) {
      if (document.view.dirty || document.pendingSave || document.pendingUndo) fail('unsaved_changes')
      const result = await invoke(document, 'open-object', {object:{workspace:document.workspace,path:document.requestedPath}})
      if (!result.ok || !('document' in result)) fail('unavailable')
      const before = result.document, bytes = decodeContent(before.content)
      if (before.objectId !== document.view.objectId || before.sessionId !== document.sessionId || before.path !== document.view.path || before.revision !== hash(bytes)) fail('invalid_response')
      if (before.revision !== document.view.revision) fail('conflict')
      document.beforeChange = {bytes, revision:before.revision}
    }
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
      if (replacement.text.length > 4096) fail('unsupported_selection')
      document.view.canUndo = false; delete document.undo
      await engine(document, {command:'replace',operationId:newOperationId(),documentId:document.view.objectId,version:scope.baseRevision,selectionToken:scope.selectionToken,expectedChangeSequence:scope.changeSequence,text:replacement.text,valueType:'text'})
      document.appliedProposals.add(proposalId)
      document.view.appliedProposals = [...document.appliedProposals].slice(-16)
      await save(document)
      return
    }
    await proposals(document,scopeId)
  }

  return {
    getView,
    restoreVisibility: async () => { visible = true; if (active?.ready && !active.pendingUndo) await active.surface.attach(active.bounds, active.appearance) },
    getViews: () => [...documents.values()].map(d => structuredClone(d.view)),
    saveAll: () => serialize(async () => { for (const document of documents.values()) if (document.view.dirty || document.pendingSave || document.pendingUndo) await save(document) }),
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
          try { await invoke(document, 'close-object', { sessionId: document.sessionId }) } catch { /* Teardown never fabricates a successful Core close. */ }
          finally {
            await destroySurface(document)
            if (active === document) active = undefined
            documents.delete(document.view.objectId)
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
      const changesVisibility = ['hide', 'open'].includes(parsed.data.action)
      const epoch = changesVisibility ? ++visibilityEpoch : visibilityEpoch
      if (parsed.data.action === 'hide') { visible = false; void active?.surface.hide(); return Promise.resolve(response()) }
      if (parsed.data.action === 'open' || parsed.data.action === 'bounds') visible = true
      return serialize(async () => {
        if (disposed) fail('unavailable')
        const request = parsed.data
        if (request.action === 'open') { await open(request, epoch); if (!visible || epoch !== visibilityEpoch) await active?.surface.hide(); return }
        if (request.action === 'close') { const target = request.objectId ? documents.get(request.objectId) : active; if (target) await close(target, request.discard); return }
        if (request.action === 'status') {
          if (active) {
            try {
              active.binding = await listBinding(active.view.kind, active.binding)
              if (active.ready && !active.pendingSave && !active.pendingUndo) { active.view.status = 'ready'; delete active.view.error; publish() }
            } catch (error) {
              setError(active, error instanceof OfficeFailure ? error.code : 'package_unavailable')
              throw error
            }
          }
          return
        }
        if (request.action === 'hide') { await active?.surface.hide(); return }
        const document = ['save','saveStatus','proposals','rejectProposal'].includes(request.action) && 'objectId' in request ? documents.get(request.objectId) : requireActive()
        if (!document) fail('unavailable')
        if (request.action === 'bounds') { if (visible && epoch === visibilityEpoch) { document.bounds = request.bounds; document.appearance = request.appearance; if (!document.pendingUndo) await document.surface.attach(request.bounds, request.appearance) } return }
        try {
          document.binding = await listBinding(document.view.kind, document.binding)
          if (request.action === 'proposals') { await proposals(document,request.scopeId); return }
          if (request.action === 'rejectProposal') { await decideProposal(document,request.scopeId,request.proposalId,false); return }
          if (request.action === 'saveStatus') { if (!document.pendingSave && !document.pendingUndo) return; await save(document); return }
          if (document.pendingUndo && request.action !== 'save') fail('unknown')
          if ('objectId' in request && (request.objectId !== document.view.objectId || !('revision' in request) || request.revision !== document.view.revision || request.expectedChangeSequence !== (document.view.changeSequence ?? 0))) fail('stale_selection')
          if (request.action === 'reference') { await reference(document,request); return }
          if (request.action === 'acceptProposal') { await decideProposal(document,request.scopeId,request.proposalId,true); return }
          if (request.action === 'undoChange') { await undoChange(document); return }
          if (request.action === 'save') { await save(document); return }
          if (request.action === 'annotate' && (!document.binding.operations.includes('commit-object') || !document.binding.operations.includes('object-status'))) fail('package_unavailable')
          if (!['captureSelection','annotate'].includes(request.action)) fail('invalid_request')
          await engine(document, { command: request.action === 'annotate' ? 'edit' : 'captureSelection', operationId: newOperationId(), documentId: document.view.objectId, version: document.view.revision })
          document.view.status = 'ready'
          delete document.view.error
          publish()
        } catch (error) {
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
