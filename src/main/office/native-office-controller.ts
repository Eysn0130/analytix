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
const token = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/)
const digest = z.string().regex(/^[a-f0-9]{64}$/)
const sequence = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
const engineStateSchema = z.object({
  documentId: digest, version: digest, kind: z.enum(['docx', 'xlsx', 'pptx']),
  changeSequence: sequence, acknowledgedSequence: sequence, dirty: z.boolean()
}).strict().refine(s => s.acknowledgedSequence <= s.changeSequence && s.dirty === (s.changeSequence !== s.acknowledgedSequence))
type EngineState = z.infer<typeof engineStateSchema>
const engineResultSchema = z.object({
  type: z.literal('result'), command: z.enum(['open', 'captureSelection', 'close']),
  operationId: token, documentId: digest, version: digest, ok: z.literal(true),
  state: engineStateSchema.optional(), selection: nativeOfficeSelectionSchema.optional()
}).strict()
const eventBase = { documentId: digest, version: digest, operationId: token }
const engineEventSchema = z.discriminatedUnion('type', [
  z.object({ ...eventBase, type: z.literal('changed'), state: engineStateSchema }).strict(),
  z.object({ ...eventBase, type: z.literal('selection'), selection: nativeOfficeSelectionSchema }).strict(),
  z.object({ ...eventBase, type: z.literal('save-requested') }).strict()
])

// Legacy transport variants remain solely for surface type compatibility. The
// controller constructs only PreviewEngineRequest and cannot initiate mutation.
export type NativeOfficeEngineRequest = {
  operationId: string; documentId: string; version: string
} & (
  | { command: 'open'; kind: NativeOfficeKind; bytes: Uint8Array }
  | { command: 'export' | 'captureSelection' | 'bold' | 'undo' | 'redo' }
  | { command: 'close'; expectedChangeSequence: number; discard: boolean }
  | { command: 'ack'; status: 'committed' | 'conflict' | 'failed'; persistedVersion?: string }
)
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
  binding: PluginPackageView; surface: NativeOfficeSurface; openOperationId: string
  ready: boolean; failed: boolean; destroyed?: Promise<void>
}
type PreviewEngineRequest = NativeOfficeEngineRequest & { command: 'open' | 'captureSelection' | 'close' }
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

/** Single-document, read-only Main preview. No save protocol or Agent/history API. */
export function createNativeOfficeController(options: NativeOfficeControllerOptions) {
  let active: Active | undefined
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
    document.view.dirty = false
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

  async function invoke(document: { binding: PluginPackageView; view: Pick<NativeOfficeView, 'kind'> }, operation: typeof operations[number], input: Record<string, unknown>): Promise<ObjectEditingResponse> {
    // Relist only the same generation. This never enables or installs a plugin.
    document.binding = await listBinding(document.view.kind, document.binding)
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
    const inner = objectEditingResponseSchema.safeParse(outer.data.output)
    if (!inner.success) fail('invalid_response')
    return inner.data
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
    void destroySurface(document)
  }
  function acceptState(document: Active, state: EngineState) {
    if (state.documentId !== document.view.objectId || state.version !== document.view.revision || state.kind !== document.view.kind ||
        state.dirty || state.changeSequence !== 0 || state.acknowledgedSequence !== 0) fail('invalid_response')
  }
  function acceptSelection(document: Active, selection: NativeOfficeSelection) {
    const expectedKind = { docx: 'text', xlsx: 'cells', pptx: 'shapes' }[document.view.kind]
    if (selection.documentId !== document.view.objectId || selection.version !== document.view.revision || selection.changeSequence !== 0 ||
        (selection.kind !== expectedKind && selection.kind !== 'unavailable')) fail('invalid_response')
    document.view.selection = selection
  }
  async function engine(document: Active, request: PreviewEngineRequest) {
    if (disposed || document.failed) fail('engine_unavailable')
    let raw: unknown
    try { raw = await document.surface.request(request) } catch { fail('engine_unavailable') }
    if (disposed) fail('engine_unavailable')
    if (document.failed) fail('invalid_response')
    const parsed = engineResultSchema.safeParse(withoutChannel(raw))
    if (!parsed.success) fail('invalid_response')
    const value = parsed.data
    if (value.command !== request.command || value.operationId !== request.operationId || value.documentId !== request.documentId || value.version !== request.version) fail('invalid_response')
    if (request.command === 'close') {
      if (value.state || value.selection) fail('invalid_response')
      return
    }
    if (!value.state) fail('invalid_response')
    acceptState(document, value.state)
    if (request.command === 'captureSelection' && !value.selection) fail('invalid_response')
    if (value.selection) acceptSelection(document, value.selection)
  }

  function onEvent(document: Active, raw: unknown) {
    if (disposed || active !== document || document.failed) return
    const fatal = z.object({ type: z.literal('fatal'), code: z.enum(['office-startup-failed', 'office-assets-unavailable', 'office-protocol-invalid', 'office-operation-timeout-unknown', 'office-engine-closed-unknown']) }).strict().safeParse(raw)
    if (fatal.success) { invalidatePreview(document, 'engine_unavailable'); return }
    const value = withoutChannel(raw)
    // Save shortcuts/buttons have no meaning in a read-only preview.
    if (value && typeof value === 'object' && 'type' in value && value.type === 'save-requested') return
    // Stale/foreign-document events cannot change the active preview.
    if (value && typeof value === 'object' && 'documentId' in value && 'version' in value &&
        (value.documentId !== document.view.objectId || value.version !== document.view.revision)) return
    const parsed = engineEventSchema.safeParse(value)
    if (!parsed.success) { invalidatePreview(document, 'invalid_response'); return }
    const event = parsed.data
    if (event.type === 'changed') { invalidatePreview(document, 'invalid_response'); return }
    if (event.operationId !== document.openOperationId) return
    if (event.type !== 'selection') return
    try { acceptSelection(document, event.selection); publish() }
    catch { invalidatePreview(document, 'invalid_response') }
  }

  async function open(request: Extract<ReturnType<typeof nativeOfficeRequestSchema.parse>, { action: 'open' }>) {
    if (!isAbsolute(request.workspace)) fail('invalid_request')
    const kind = extname(request.path).slice(1).toLowerCase() as NativeOfficeKind
    if (!Object.hasOwn(packageIds, kind)) fail('invalid_request')
    if (active) {
      if (active.ready && active.workspace === request.workspace && active.requestedPath === request.path) {
        active.binding = await listBinding(active.view.kind, active.binding)
        await active.surface.attach(request.bounds, request.appearance)
        active.view.status = 'ready'
        delete active.view.error
        publish()
        return
      }
      await close()
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
      const bytes = decodeContent(opened.content)
      if (!isAbsolute(opened.path) || resolve(opened.path) !== resolve(request.workspace, request.path) || opened.revision !== hash(bytes) || extname(opened.path).toLowerCase() !== '.' + kind) fail('invalid_response')
      if (disposed) fail('unavailable')
      surface = await options.createSurface(event => { if (document) onEvent(document, event) })
      if (disposed) fail('unavailable')
      document = { view: { objectId: opened.objectId, path: opened.path, kind, revision: opened.revision, dirty: false, status: 'loading' },
        workspace: request.workspace, requestedPath: request.path, sessionId: opened.sessionId,
        binding: holder.binding, surface, openOperationId: newOperationId(), ready: false, failed: false }
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
      if (active === document) active = undefined
      publish()
      throw error
    }
  }

  async function close() {
    const document = active
    if (!document) return
    // Read-only previews have no work to save or discard. Close the engine if
    // usable, then revoke Core and destroy resources even after loading failure.
    if (document.ready && !document.failed) {
      try {
        await engine(document, { command: 'close', documentId: document.view.objectId, version: document.view.revision,
          operationId: newOperationId(), expectedChangeSequence: 0, discard: true })
      } catch { /* Resource destruction below also closes an unavailable engine. */ }
    }
    document.ready = false
    try {
      const result = await invoke(document, 'close-object', { sessionId: document.sessionId })
      if (!result.ok || !('closed' in result)) fail('unavailable')
    } finally {
      await destroySurface(document)
      if (active === document) active = undefined
      publish()
    }
  }

  return {
    getView,
    /** Owner teardown releases read-only resources. */
    destroy(): Promise<void> {
      if (shutdown) return shutdown
      disposed = true
      const closing = active
      // Destroy first to reject any in-flight preview request; serialize Core
      // cleanup behind the pending open/capture operation.
      shutdown = Promise.resolve().then(() => closing && destroySurface(closing)).then(async () => {
        await serialize(async () => {
          const document = active
          if (!document) return
          try { await invoke(document, 'close-object', { sessionId: document.sessionId }) } catch { /* Teardown never fabricates a successful Core close. */ }
          finally {
            await destroySurface(document)
            if (active === document) active = undefined
            publish()
          }
        })
      })
      return shutdown
    },
    request(payload: unknown): Promise<NativeOfficeResponse> {
      if (disposed) return Promise.resolve(response('unavailable'))
      if (payload && typeof payload === 'object' && 'action' in payload &&
          typeof payload.action === 'string' && ['save', 'bold', 'undo', 'redo'].includes(payload.action)) return Promise.resolve(response('invalid_request'))
      const parsed = nativeOfficeRequestSchema.safeParse(payload)
      if (!parsed.success) return Promise.resolve(response('invalid_request'))
      return serialize(async () => {
        if (disposed) fail('unavailable')
        const request = parsed.data
        if (request.action === 'open') { await open(request); return }
        if (request.action === 'close') { await close(); return }
        if (request.action === 'status') {
          if (active) {
            try {
              active.binding = await listBinding(active.view.kind, active.binding)
              if (active.ready) { active.view.status = 'ready'; delete active.view.error; publish() }
            } catch (error) {
              setError(active, error instanceof OfficeFailure ? error.code : 'package_unavailable')
              throw error
            }
          }
          return
        }
        if (request.action === 'hide') { await active?.surface.hide(); return }
        const document = requireActive()
        if (request.action === 'bounds') { await document.surface.attach(request.bounds, request.appearance); return }
        try {
          document.binding = await listBinding(document.view.kind, document.binding)
          if (request.action !== 'captureSelection') fail('invalid_request')
          await engine(document, { command: 'captureSelection', operationId: newOperationId(), documentId: document.view.objectId, version: document.view.revision })
          document.view.status = 'ready'
          delete document.view.error
          publish()
        } catch (error) {
          const code = error instanceof OfficeFailure ? error.code : 'engine_unavailable'
          if (code === 'invalid_response' || code === 'engine_unavailable') invalidatePreview(document, code)
          else setError(document, code)
          throw error
        }
      })
    }
  }
}
