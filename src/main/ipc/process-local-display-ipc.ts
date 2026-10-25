import { ipcMain, shell, type BrowserWindow, type IpcMainInvokeEvent } from 'electron'
import { randomUUID } from 'node:crypto'
import type { ZodType } from 'zod'
import {
  PROCESS_LOCAL_DISPLAY_READ_PATH_V1, PROCESS_LOCAL_DISPLAY_TARGET_PATH_V1,
  PROCESS_LOCAL_DISPLAY_MAX_BYTES_V1,
  processDisplayAuthorizeResultSchemaV1, processDisplayReadResultSchemaV1, processDisplayTargetResultSchemaV1,
  processPaneAuthorizeRequestSchemaV1, processPaneReadRequestSchemaV1, processPaneExpansionRequestSchemaV1,
  processPaneViewerRequestSchemaV1, processPaneTargetRequestSchemaV1, processPaneWebLinkRequestSchemaV1,
  processDisplayWebLinkResultSchemaV1,
  hasProcessDisplayPayloadV2,
  type ProcessDisplayScopeV1, type ProcessPaneAuthorizeResultV1, type ProcessPaneReadResultV1,
  type ProcessPaneActionResultV1, type ProcessPaneTargetResultV1
} from '../../../packages/runtime/src/contracts/protected-process-local-display'
import { captureNativeRendererDocument } from './native-renderer-document'

export type ProcessLocalDisplayTransportV1 = (path: string, body: string, signal?: AbortSignal) =>
  Promise<{ ok: boolean; status: number; body: string }>
/** Main-only closure over one captured managed-runtime authority pin. */
export type ProcessLocalDisplayRuntimeOwnerV1 = { isCurrent: () => boolean }
export type ProcessLocalDisplayDocumentOwnerV1 = {
  ownerId: number; documentNonce: string; documentEpoch?: string; signal: AbortSignal
  onAuthorityLost?: () => void
  isCurrent: () => boolean; release: () => void
}
type Ticket = { controller: AbortController; epoch: number }
type Viewer = {
  viewerId: string; viewerToken: string; paneKey: string
  scope: { threadId: string; turnId: string; contextDigest?: string; contextEpoch?: string; processIncarnation?: string }
  owner: ProcessLocalDisplayDocumentOwnerV1; runtime: ProcessLocalDisplayRuntimeOwnerV1
  state: 'authorizing' | 'active' | 'revoked'; epoch: number
  expanded: Set<string>; draftIds: Set<string>; revision: string; poll: Ticket | null; target: Ticket | null
}
const registeredInvalidators = new Set<() => void>()
let transitionDepth = 0
const unavailable = (): { status: 'unavailable' } => ({ status: 'unavailable' })
const safeCurrent = (current: () => boolean): boolean => { try { return current() } catch { return false } }
const sameScope = (actual: ProcessDisplayScopeV1, expected: Viewer['scope']): boolean =>
  actual.threadId === expected.threadId && actual.turnId === expected.turnId &&
  (expected.contextDigest === undefined || actual.contextDigest === expected.contextDigest) &&
  (expected.contextEpoch === undefined || actual.contextEpoch === expected.contextEpoch) &&
  (expected.processIncarnation === undefined || actual.processIncarnation === expected.processIncarnation)
const revisionAtLeast = (actual: string, previous: string): boolean => BigInt(actual) >= BigInt(previous)

export function invalidateProcessLocalDisplaysV1(): void {
  for (const invalidate of [...registeredInvalidators]) {
    try { invalidate() } catch { /* Continue retiring other native document owners. */ }
  }
}
export async function withProcessLocalDisplayTransitionV1<T>(operation: () => Promise<T>): Promise<T> {
  transitionDepth += 1
  try { invalidateProcessLocalDisplaysV1(); return await operation() }
  finally {
    try { invalidateProcessLocalDisplaysV1() }
    finally { transitionDepth -= 1 }
  }
}

/** One lease per document/pane. Only identities, epochs and revision are retained;
 * private response copies exist only in the active request and protected leaf. */
export class ProcessLocalDisplayHostV1 {
  private viewers = new Map<string, Viewer>()
  private panes = new Map<string, Viewer>()
  constructor(
    private readonly transport: ProcessLocalDisplayTransportV1,
    private readonly captureRuntimeOwner: () => ProcessLocalDisplayRuntimeOwnerV1 | null
  ) {}
  private current(view: Viewer): boolean {
    if (view.state === 'revoked' || this.viewers.get(view.viewerId) !== view || this.panes.get(view.paneKey) !== view || view.owner.signal.aborted) return false
    if (!safeCurrent(view.owner.isCurrent) || !safeCurrent(view.runtime.isCurrent)) {
      try { view.owner.onAuthorityLost?.() } catch { /* Retire locally even if notification fails. */ }
      return false
    }
    return true
  }
  private cancelBodyReads(view: Viewer): void {
    view.epoch += 1
    const poll = view.poll, target = view.target
    view.poll = null; view.target = null
    poll?.controller.abort(); target?.controller.abort()
  }
  private revokeGo(view: Viewer): void {
    if (!safeCurrent(view.runtime.isCurrent)) return
    // This never returns display bytes. The local lease is already permanently
    // retired even if the remote revoke cannot be acknowledged.
    void this.transport(PROCESS_LOCAL_DISPLAY_READ_PATH_V1,
      JSON.stringify({ schemaVersion: 2, action: 'revoke', viewerToken: view.viewerToken }), AbortSignal.timeout(1000)).catch(() => undefined)
  }
  private retire(view: Viewer): void {
    if (view.state === 'revoked') return
    view.state = 'revoked'; view.expanded.clear(); view.draftIds.clear(); this.cancelBodyReads(view)
    if (this.viewers.get(view.viewerId) === view) this.viewers.delete(view.viewerId)
    if (this.panes.get(view.paneKey) === view) this.panes.delete(view.paneKey)
    try { view.owner.release() } catch { /* An irreversible lease cannot recover. */ }
    this.revokeGo(view)
  }
  invalidate(): void { for (const view of [...this.viewers.values()]) this.retire(view) }
  revokeDocument(documentNonce: string): void {
    for (const view of [...this.viewers.values()]) if (view.owner.documentNonce === documentNonce) this.retire(view)
  }
  revokeOwner(ownerId: number): void {
    for (const view of [...this.viewers.values()]) if (view.owner.ownerId === ownerId) this.retire(view)
  }
  private lookup(viewerId: string, ownerId: number): Viewer | null {
    const view = this.viewers.get(viewerId)
    if (!view || view.owner.ownerId !== ownerId || view.state !== 'active') return null
    if (!this.current(view)) { this.retire(view); return null }
    return view
  }
  private async request<T>(path: string, body: unknown, schema: ZodType<T>, signal: AbortSignal): Promise<T | null> {
    try {
      if (signal.aborted) return null
      const response = await this.transport(path, JSON.stringify(body), signal)
      if (signal.aborted || !response.ok || response.status !== 200 ||
          Buffer.byteLength(response.body, 'utf8') > PROCESS_LOCAL_DISPLAY_MAX_BYTES_V1) return null
      const parsed = schema.safeParse(JSON.parse(response.body))
      return parsed.success ? parsed.data : null
    } catch { return null }
  }
  async authorize(payload: unknown, owner: ProcessLocalDisplayDocumentOwnerV1): Promise<ProcessPaneAuthorizeResultV1> {
    const parsed = processPaneAuthorizeRequestSchemaV1.safeParse(payload)
    if (!parsed.success || !safeCurrent(owner.isCurrent) || owner.signal.aborted || transitionDepth !== 0) {
      owner.release(); return unavailable()
    }
    const paneKey = String(owner.ownerId) + ':' + (owner.documentEpoch ?? owner.documentNonce) + ':' + parsed.data.paneId
    const previous = this.panes.get(paneKey)
    if (previous) this.retire(previous)
    let runtime: ProcessLocalDisplayRuntimeOwnerV1 | null
    try { runtime = this.captureRuntimeOwner() } catch { runtime = null }
    if (!runtime || !safeCurrent(runtime.isCurrent) || this.viewers.size >= 8) { owner.release(); return unavailable() }
    const view: Viewer = {
      viewerId: randomUUID(), viewerToken: randomUUID(), paneKey,
      scope: { threadId: parsed.data.threadId, turnId: parsed.data.turnId }, owner, runtime,
      state: 'authorizing', epoch: 0, expanded: new Set(), draftIds: new Set(), revision: '0', poll: null, target: null
    }
    this.viewers.set(view.viewerId, view); this.panes.set(paneKey, view)
    const response = await this.request(PROCESS_LOCAL_DISPLAY_READ_PATH_V1, {
      schemaVersion: 2, action: 'authorize', viewerToken: view.viewerToken, documentNonce: owner.documentNonce,
      threadId: view.scope.threadId, turnId: view.scope.turnId
    }, processDisplayAuthorizeResultSchemaV1, AbortSignal.any([owner.signal, AbortSignal.timeout(30_000)]))
    if (!this.current(view)) { this.retire(view); this.revokeGo(view); return unavailable() }
    if (!response) { this.retire(view); return unavailable() }
    if (response.status !== 'authorized') { this.retire(view); return response }
    if (!sameScope(response.scope, view.scope)) { this.retire(view); return unavailable() }
    view.scope = response.scope; view.revision = response.revision; view.state = 'active'
    // No await after this final document/pane/runtime check.
    if (!this.current(view)) { this.retire(view); return unavailable() }
    return { ...response, viewerId: view.viewerId }
  }
  async read(payload: unknown, ownerId: number): Promise<ProcessPaneReadResultV1> {
    const parsed = processPaneReadRequestSchemaV1.safeParse(payload)
    if (!parsed.success) return unavailable()
    const view = this.lookup(parsed.data.viewerId, ownerId)
    if (!view) return unavailable()
    if (view.poll) return { status: 'busy' }
    const ticket: Ticket = { controller: new AbortController(), epoch: view.epoch }
    view.poll = ticket
    const expandedBlockIds = [...view.expanded].filter(id => !parsed.data.draftsOnly || view.draftIds.has(id))
    try {
      const wireResponse = await this.request(PROCESS_LOCAL_DISPLAY_READ_PATH_V1, {
        schemaVersion: 2, action: 'read', viewerToken: view.viewerToken, expandedBlockIds,
        waitMs: parsed.data.waitMs ?? 0,
        ...(parsed.data.afterRevision === undefined ? {} : { afterRevision: parsed.data.afterRevision })
      }, processDisplayReadResultSchemaV1, AbortSignal.any([view.owner.signal, ticket.controller.signal, AbortSignal.timeout(30_000)]))
      // Source-only discovery does not reveal a folded native/tool preview or
      // body. The projection is Main-owned; no renderer-supplied text enters it.
      const response = wireResponse?.status === 'snapshot' && parsed.data.draftsOnly
        ? { ...wireResponse, blocks: wireResponse.blocks.filter(block => block.source.kind === 'assistant_draft'),
          units: wireResponse.units.filter(unit => wireResponse.blocks.some(block => block.source.kind === 'assistant_draft' && block.source.unitId === unit.unitId)) }
        : wireResponse
      if (view.poll !== ticket || ticket.epoch !== view.epoch || ticket.controller.signal.aborted) return unavailable()
      if (!this.current(view)) { this.retire(view); return unavailable() }
      if (!response) { this.retire(view); return unavailable() }
      if (response.status !== 'snapshot' && response.status !== 'unchanged') {
        if (response.status !== 'busy') this.retire(view)
        return response
      }
      if (!sameScope(response.scope, view.scope) || !revisionAtLeast(response.revision, view.revision) ||
          wireResponse?.status === 'snapshot' && wireResponse.blocks.some(block => hasProcessDisplayPayloadV2(block) && !expandedBlockIds.includes(block.blockId))) {
        this.retire(view); return unavailable()
      }
      if (wireResponse?.status === 'snapshot') view.draftIds = new Set(wireResponse.blocks.filter(block => block.source.kind === 'assistant_draft').map(block => block.blockId))
      view.revision = response.revision
      if (!this.current(view) || view.poll !== ticket || ticket.epoch !== view.epoch) { this.retire(view); return unavailable() }
      return response
    } finally { if (view.poll === ticket) view.poll = null }
  }
  setExpanded(payload: unknown, ownerId: number): ProcessPaneActionResultV1 {
    const parsed = processPaneExpansionRequestSchemaV1.safeParse(payload)
    if (!parsed.success) return unavailable()
    const view = this.lookup(parsed.data.viewerId, ownerId)
    if (!view) return unavailable()
    // Collapse clears expansion and cancels body reads, retaining the preview
    // lease and Go authorization. Re-expand uses the same valid pane owner.
    view.expanded = new Set(parsed.data.expandedBlockIds); this.cancelBodyReads(view)
    return { status: 'ok' }
  }
  close(payload: unknown, ownerId: number): ProcessPaneActionResultV1 {
    const parsed = processPaneViewerRequestSchemaV1.safeParse(payload)
    if (!parsed.success) return unavailable()
    const view = this.lookup(parsed.data.viewerId, ownerId)
    if (!view) return unavailable()
    this.retire(view); return { status: 'ok' }
  }
  async openWebLink(payload: unknown, ownerId: number, openExternal: (url: string) => Promise<void>): Promise<ProcessPaneActionResultV1> {
    const parsed = processPaneWebLinkRequestSchemaV1.safeParse(payload)
    if (!parsed.success) return unavailable()
    const view = this.lookup(parsed.data.viewerId, ownerId)
    if (!view) return unavailable()
    if (view.target) return { status: 'busy' }
    const ticket: Ticket = { controller: new AbortController(), epoch: view.epoch }
    view.target = ticket
    const validTicket = () => view.target === ticket && ticket.epoch === view.epoch && !ticket.controller.signal.aborted && this.current(view)
    const wire = { schemaVersion: 2, action: 'open_web_link', viewerToken: view.viewerToken,
      blockId: parsed.data.blockId, inputDigest: parsed.data.inputDigest, role: parsed.data.role }
    const matches = (response: { scope: ProcessDisplayScopeV1; revision: string; blockId: string; inputDigest: string; role: string }) =>
      sameScope(response.scope, view.scope) && revisionAtLeast(response.revision, view.revision) &&
      response.blockId === parsed.data.blockId && response.inputDigest === parsed.data.inputDigest && response.role === parsed.data.role
    try {
      const signal = AbortSignal.any([view.owner.signal, ticket.controller.signal, AbortSignal.timeout(30_000)])
      const first = await this.request(PROCESS_LOCAL_DISPLAY_TARGET_PATH_V1, wire, processDisplayWebLinkResultSchemaV1, signal)
      if (!validTicket()) return unavailable()
      if (!first) { this.retire(view); return unavailable() }
      if (first.status !== 'web_link') {
        if (first.status !== 'busy') this.retire(view)
        return first
      }
      if (!matches(first)) { this.retire(view); return unavailable() }
      const final = await this.request(PROCESS_LOCAL_DISPLAY_TARGET_PATH_V1, wire, processDisplayWebLinkResultSchemaV1, signal)
      if (!validTicket()) return unavailable()
      if (!final) { this.retire(view); return unavailable() }
      if (final.status !== 'web_link') {
        if (final.status !== 'busy') this.retire(view)
        return final
      }
      if (!matches(final) || !revisionAtLeast(final.revision, first.revision) ||
          final.targetUrl !== first.targetUrl) { this.retire(view); return unavailable() }
      view.revision = final.revision
      // The active input target is resolved by Go under this existing viewer
      // lease. The renderer contributes only identity, digest and role.
      const url = new URL(final.targetUrl)
      if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !validTicket()) { this.retire(view); return unavailable() }
      await openExternal(final.targetUrl)
      return { status: 'ok' }
    } catch { return unavailable() }
    finally { if (view.target === ticket) view.target = null }
  }
  async openCapture(payload: unknown, ownerId: number): Promise<ProcessPaneTargetResultV1> {
    const parsed = processPaneTargetRequestSchemaV1.safeParse(payload)
    if (!parsed.success) return unavailable()
    const view = this.lookup(parsed.data.viewerId, ownerId)
    if (!view || !view.expanded.has(parsed.data.blockId)) return unavailable()
    if (view.target) return { status: 'busy' }
    const ticket: Ticket = { controller: new AbortController(), epoch: view.epoch }
    view.target = ticket
    try {
      const response = await this.request(PROCESS_LOCAL_DISPLAY_TARGET_PATH_V1, {
        schemaVersion: 2, action: 'open_capture', viewerToken: view.viewerToken, blockId: parsed.data.blockId, captureDigest: parsed.data.captureDigest
      }, processDisplayTargetResultSchemaV1, AbortSignal.any([view.owner.signal, ticket.controller.signal, AbortSignal.timeout(30_000)]))
      if (view.target !== ticket || ticket.epoch !== view.epoch || ticket.controller.signal.aborted) return unavailable()
      if (!this.current(view)) { this.retire(view); return unavailable() }
      if (!response) { this.retire(view); return unavailable() }
      if (response.status !== 'capture') {
        if (response.status !== 'busy') this.retire(view)
        return response
      }
      if (!sameScope(response.scope, view.scope) || !revisionAtLeast(response.revision, view.revision) ||
          response.block.blockId !== parsed.data.blockId || response.block.captureDigest !== parsed.data.captureDigest || response.block.body === undefined) {
        this.retire(view); return unavailable()
      }
      view.revision = response.revision
      if (!this.current(view) || view.target !== ticket || ticket.epoch !== view.epoch) { this.retire(view); return unavailable() }
      return response
    } finally { if (view.target === ticket) view.target = null }
  }
}

export function registerProcessLocalDisplayIpcV1(
  getMainWindow: () => BrowserWindow | null,
  transport: ProcessLocalDisplayTransportV1,
  captureRuntimeOwner: () => ProcessLocalDisplayRuntimeOwnerV1 | null,
  trustedSender?: (event: IpcMainInvokeEvent) => boolean
): void {
  const host = new ProcessLocalDisplayHostV1(transport, captureRuntimeOwner)
  const observedWindows = new WeakSet<BrowserWindow>()
  type DocumentRecord = { event: IpcMainInvokeEvent; documentEpoch: string; lease: ReturnType<typeof captureNativeRendererDocument> | null; authorityLost: boolean; releaseNavigation: () => void }
  let documentRecord: DocumentRecord | null = null
  let documentSequence = 0n, authorityEpoch = 0n
  const nativeCurrent = (event: IpcMainInvokeEvent): boolean => {
    const window = getMainWindow()
    return Boolean(window && !window.isDestroyed() && window.webContents === event.sender &&
      !event.sender.isDestroyed() && event.senderFrame != null && event.senderFrame === event.sender.mainFrame)
  }
  const stamps = (record: DocumentRecord) => ({ schemaVersion: 2 as const,
    documentEpoch: record.documentEpoch, authorityEpoch: String(authorityEpoch) })
  const notifyDocumentInvalidated = (record: DocumentRecord | null): void => {
    if (!record || documentRecord !== record || !safeCurrent(() => nativeCurrent(record.event)) || !record.lease?.isCurrent()) return
    try { record.event.senderFrame!.send('process-local:invalidated', stamps(record)) } catch { /* Local leases stay retired. */ }
  }
  const invalidate = () => {
    authorityEpoch += 1n
    if (documentRecord) documentRecord.authorityLost = true
    host.invalidate()
    // Independent document watch survives the last pane's close and a global
    // authority change. Its native document identity remains irreversible.
    notifyDocumentInvalidated(documentRecord)
  }
  registeredInvalidators.add(invalidate)
  const current = (event: IpcMainInvokeEvent): boolean => {
    const window = getMainWindow()
    return Boolean(transitionDepth === 0 && nativeCurrent(event) && window?.isVisible() && !window.isMinimized() && (!trustedSender || trustedSender(event)))
  }
  const guard = (event: IpcMainInvokeEvent): boolean => {
    const valid = safeCurrent(() => current(event))
    if (!valid && documentRecord?.event.sender === event.sender && event.senderFrame != null && event.senderFrame === documentRecord.event.senderFrame && !documentRecord.authorityLost) invalidate()
    return valid
  }
  const captureDocument = (event: IpcMainInvokeEvent): DocumentRecord | null => {
    const existing = documentRecord
    if (existing && existing.event.sender === event.sender && existing.event.senderFrame === event.senderFrame && existing.lease?.isCurrent()) return existing
    if (existing) { documentRecord = null; existing.releaseNavigation(); existing.lease?.release(); host.invalidate() }
    documentSequence += 1n
    const record: DocumentRecord = { event, documentEpoch: String(documentSequence), lease: null, authorityLost: false, releaseNavigation: () => undefined }
    documentRecord = record
    // In-place native navigation can leave the same DOM/preload alive. Send its
    // old-owner fence before the irreversible document helper retires the watch.
    const navigating = (_event: Electron.Event, _url: string, inPlace: boolean, mainFrame: boolean) => {
      if (inPlace && mainFrame && documentRecord === record) invalidate()
    }
    event.sender.on('did-start-navigation', navigating)
    record.releaseNavigation = () => { event.sender.removeListener('did-start-navigation', navigating) }
    record.lease = captureNativeRendererDocument(event.sender, () => nativeCurrent(event), () => {
      record.releaseNavigation()
      if (documentRecord !== record) return
      documentRecord = null; authorityEpoch += 1n; host.revokeOwner(event.sender.id)
    })
    return record.lease.isCurrent() ? record : null
  }
  const observeWindow = () => {
    const window = getMainWindow()
    if (!window || observedWindows.has(window)) return
    observedWindows.add(window)
    window.on('hide', invalidate); window.on('minimize', invalidate)
    window.once('closed', () => {
      invalidate(); window.removeListener('hide', invalidate); window.removeListener('minimize', invalidate)
    })
  }
  ipcMain.handle('process-local:authorize', async (event, payload: unknown) => {
    if (!guard(event)) return unavailable()
    if (!processPaneAuthorizeRequestSchemaV1.safeParse(payload).success) return unavailable()
    observeWindow()
    const record = captureDocument(event)
    if (!record) return unavailable()
    record.authorityLost = false
    const control = stamps(record)
    const documentNonce = randomUUID(), controller = new AbortController()
    const lease = captureNativeRendererDocument(event.sender, () => current(event) && documentRecord === record && record.lease!.isCurrent(), () => {
      controller.abort(); host.revokeDocument(documentNonce)
      if (documentRecord === record && !record.authorityLost && !safeCurrent(() => current(event))) invalidate()
    })
    const result = await host.authorize(payload, {
      ownerId: event.sender.id, documentNonce, documentEpoch: record.documentEpoch,
      signal: controller.signal, isCurrent: lease.isCurrent, onAuthorityLost: () => { if (documentRecord === record && !record.authorityLost) invalidate() },
      release: () => { controller.abort(); lease.release() }
    })
    return { control, result }

  })
  ipcMain.handle('process-local:read', (event, payload: unknown) =>
    guard(event) ? host.read(payload, event.sender.id) : unavailable())
  ipcMain.handle('process-local:expand', (event, payload: unknown) =>
    guard(event) ? host.setExpanded(payload, event.sender.id) : unavailable())
  ipcMain.handle('process-local:close', (event, payload: unknown) =>
    guard(event) ? host.close(payload, event.sender.id) : unavailable())
  ipcMain.handle('process-local:web-link', (event, payload: unknown) =>
    guard(event) ? host.openWebLink(payload, event.sender.id, url => shell.openExternal(url)) : unavailable())
  ipcMain.handle('process-local:target', (event, payload: unknown) =>
    guard(event) ? host.openCapture(payload, event.sender.id) : unavailable())
}
