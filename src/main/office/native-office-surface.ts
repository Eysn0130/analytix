import { app, MessageChannelMain, session, WebContentsView, type BrowserWindow, type MessagePortMain, type Rectangle, type Session } from 'electron'
import type { NativeOfficeAppearance } from '../../shared/native-office'
import { nativeWorkspaceCommandFromInput } from '../../shared/native-office'
import { randomUUID } from 'node:crypto'
import { lstat, realpath } from 'node:fs/promises'
import { isAbsolute, resolve } from 'node:path'
import { loadOfficeBuffers, startOfficeAssetServer, type OfficeAssetServer } from './office-assets'
import { OFFICE_BIND_IPC, isOfficeEvent, isOfficeEngineFailure, isOfficeReady, isOfficeRequest, isOfficeResult, officeRecord, sameOfficeEnvelope, type OfficeEvent, type OfficeRequest, type OfficeRequestInput, type OfficeResult, type OfficeState } from './office-protocol'

export type OfficeSurfaceFatal = { type: 'fatal'; code: 'office-startup-failed' | 'office-assets-unavailable' | 'office-protocol-invalid' | 'office-operation-timeout-unknown' | 'office-engine-closed-unknown' }
export type NativeOfficeSurfaceOptions = {
  owner: BrowserWindow
  assetRoot: string
  sourceRoot: string
  preloadPath: string
  onEvent: (event: OfficeEvent | OfficeSurfaceFatal) => void
}
type Pending = { request: OfficeRequest; resolve: (result: OfficeResult) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout> }
const failure = (code: string) => new Error(code)
const validBounds = (bounds: Rectangle) => ['x', 'y', 'width', 'height'].every(k => Number.isSafeInteger(bounds[k as keyof Rectangle]) && bounds[k as keyof Rectangle] >= 0 && bounds[k as keyof Rectangle] <= 16384) && bounds.width > 0 && bounds.height > 0

// Main-owned source-experiment surface. The caller retains Core document authority.
export class NativeOfficeSurface {
  private readonly channel = randomUUID()
  private view?: WebContentsView
  private officeSession?: Session
  private server?: OfficeAssetServer
  private port?: MessagePortMain
  private starting?: Promise<void>
  private ready = false
  private attached = false
  private appearanceKey?: string
  private appearanceCSS?: string
  private visibilityEpoch = 0
  private destroyed = false
  private terminalCode = 'office-surface-destroyed'
  private current?: OfficeState
  private openOperationId?: string
  private editing = false
  private pending?: Pending
  private seen = new Set<string>()
  private readyResolve?: () => void
  private readyReject?: (error: Error) => void
  private readyTimer?: ReturnType<typeof setTimeout>
  private readonly ownerClosed = () => this.destroy()

  constructor(private readonly options: NativeOfficeSurfaceOptions) {
    if (app.isPackaged) throw failure('office-source-experiment-only')
    if (options.owner.isDestroyed()) throw failure('office-owner-unavailable')
    options.owner.once('closed', this.ownerClosed)
  }

  async attach(bounds: Rectangle, appearance?: NativeOfficeAppearance): Promise<void> {
    if (!validBounds(bounds)) throw failure('office-invalid-bounds')
    const epoch = ++this.visibilityEpoch
    await this.start()
    if (epoch !== this.visibilityEpoch) return
    if (this.destroyed || this.options.owner.isDestroyed()) throw failure('office-surface-destroyed')
    if (!this.attached) { this.options.owner.contentView.addChildView(this.view!); this.attached = true }
    // Renderer rectangles are CSS pixels; native child views use window DIPs.
    const zoom = this.options.owner.webContents.getZoomFactor()
    this.view!.setBounds({ x: Math.round(bounds.x * zoom), y: Math.round(bounds.y * zoom), width: Math.round(bounds.width * zoom), height: Math.round(bounds.height * zoom) })
    if (appearance) await this.applyAppearance(appearance)
    if (epoch === this.visibilityEpoch && !this.destroyed) this.view!.setVisible(true)
  }

  private async applyAppearance(appearance: NativeOfficeAppearance): Promise<void> {
    const key = `${appearance.theme}:${appearance.reducedMotion}`
    if (key === this.appearanceKey) return
    const contents = this.view!.webContents
    // Only closed, validated appearance values reach CSS; document pixels are untouched.
    const colors = appearance.theme === 'dark'
      ? '--preview-bg:#1d1f23;--preview-bar:#24262a;--preview-line:#393c42;--preview-text:#c1c5cd;--preview-hover:#363a42;--preview-press:#424853;--preview-focus:#adbdd4'
      : '--preview-bg:#f3f4f6;--preview-bar:#fafbfc;--preview-line:#e5e7eb;--preview-text:#5e6673;--preview-hover:#e9ecf0;--preview-press:#dde2e9;--preview-focus:#536780'
    const css = `:root{color-scheme:${appearance.theme}!important;${colors.split(';').map(rule => `${rule}!important`).join(';')}}${appearance.reducedMotion ? 'button{transition:none!important}' : ''}`
    const previous = this.appearanceCSS
    this.appearanceCSS = await contents.insertCSS(css)
    this.appearanceKey = key
    if (previous) await contents.removeInsertedCSS(previous)
  }

  hide(): void { this.visibilityEpoch++; if (!this.destroyed) this.view?.setVisible(false) }

  async request(input: OfficeRequestInput): Promise<OfficeResult> {
    if (!officeRecord(input) || Object.hasOwn(input, 'channel')) throw failure('office-invalid-request')
    const request = { ...input, channel: this.channel }
    if (!isOfficeRequest(request)) throw failure('office-invalid-request')
    if (!this.editing && !['open','edit','captureSelection','close'].includes(request.command)) throw failure('office-invalid-request')
    // Snapshot caller-owned bytes before any await or structured-clone dispatch.
    if (request.bytes) request.bytes = new Uint8Array(request.bytes)
    await this.start()
    if (this.destroyed || !this.ready) throw failure('office-surface-destroyed')
    if (this.pending) throw failure('office-operation-in-progress')
    const key = request.command + ':' + request.operationId
    if (this.seen.has(key)) throw failure('office-operation-replayed')
    if (this.seen.size >= 4096) throw failure('office-session-operation-limit')
    if (request.command === 'open') {
      if (this.current) throw failure('office-document-already-open')
    } else {
      if (!this.current || request.documentId !== this.current.documentId || request.version !== this.current.version) throw failure('office-stale-document-version')
    }
    this.seen.add(key)
    return new Promise<OfficeResult>((resolve, reject) => {
      this.pending = { request, resolve, reject, timer: setTimeout(() => this.fail('office-operation-timeout-unknown'), 65000) }
      try { this.port!.postMessage(request) } catch { this.fail('office-engine-closed-unknown') }
    })
  }

  private start(): Promise<void> {
    if (this.destroyed) return Promise.reject(failure(this.terminalCode))
    return this.starting ??= this.initialize()
  }

  private async initialize(): Promise<void> {
    try {
      const { preloadPath, sourceRoot, assetRoot } = this.options
      if (!isAbsolute(preloadPath) || resolve(preloadPath) !== preloadPath || await realpath(preloadPath) !== preloadPath) throw failure('office-assets-unavailable')
      const preloadStat = await lstat(preloadPath)
      if (!preloadStat.isFile() || preloadStat.isSymbolicLink() || preloadStat.nlink !== 1) throw failure('office-assets-unavailable')
      const buffers = await loadOfficeBuffers(sourceRoot, assetRoot)
      if (this.destroyed) { buffers.clear(); throw failure('office-surface-destroyed') }
      const server = await startOfficeAssetServer(buffers, randomUUID())
      if (this.destroyed) { server.destroy(); throw failure('office-surface-destroyed') }
      this.server = server
      this.officeSession = session.fromPartition('analytix-native-office-' + randomUUID(), { cache: false })
      this.officeSession.setPermissionCheckHandler(() => false)
      this.officeSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
      this.officeSession.setDevicePermissionHandler(() => false)
      this.officeSession.setDisplayMediaRequestHandler((_request, callback) => callback({}))
      this.officeSession.on('will-download', (event, item) => { event.preventDefault(); item.cancel() })
      this.officeSession.webRequest.onBeforeRequest((details, callback) => callback({ cancel: !this.server?.permits(details.url, details.method) }))
      this.view = new WebContentsView({ webPreferences: {
        session: this.officeSession, preload: preloadPath, sandbox: true, contextIsolation: true,
        nodeIntegration: false, nodeIntegrationInWorker: false, nodeIntegrationInSubFrames: false,
        webSecurity: true, allowRunningInsecureContent: false, webviewTag: false,
        devTools: false, spellcheck: false, navigateOnDragDrop: false, safeDialogs: true
      } })
      this.view.setVisible(false)
      const contents = this.view.webContents
      contents.on('before-input-event', (event, input) => {
        if (input.type !== 'keyDown' || input.isAutoRepeat || !this.current) return
        const command = nativeWorkspaceCommandFromInput(input)
        if (!command) return
        event.preventDefault()
        this.options.owner.webContents.send('office:workspace-command', {objectId:this.current.documentId,command})
      })
      contents.setWindowOpenHandler(() => ({ action: 'deny' }))
      contents.setWebRTCIPHandlingPolicy('disable_non_proxied_udp')
      contents.on('will-navigate', event => event.preventDefault())
      contents.on('will-frame-navigate', event => event.preventDefault())
      contents.on('will-redirect', event => event.preventDefault())
      contents.on('will-attach-webview', event => event.preventDefault())
      contents.on('context-menu', event => event.preventDefault())
      contents.on('will-prevent-unload', event => event.preventDefault())
      contents.on('render-process-gone', () => this.fail('office-engine-closed-unknown'))
      contents.on('destroyed', () => { if (!this.destroyed) this.fail('office-engine-closed-unknown') })
      contents.on('did-navigate', (_event, url) => { if (url !== server.entryURL) this.fail('office-protocol-invalid') })
      await contents.loadURL(server.entryURL)
      if (this.destroyed) throw failure(this.terminalCode)
      const { port1, port2 } = new MessageChannelMain()
      this.port = port1
      port1.on('message', event => this.receive(event.data))
      port1.on('close', () => { if (!this.destroyed) this.fail('office-engine-closed-unknown') })
      const ready = new Promise<void>((resolve, reject) => {
        this.readyResolve = resolve; this.readyReject = reject
        this.readyTimer = setTimeout(() => this.fail('office-operation-timeout-unknown'), 65000)
      })
      void ready.catch(() => {}) // Also handled if postMessage throws before the await below.
      port1.start()
      contents.postMessage(OFFICE_BIND_IPC, { channel: this.channel, entryURL: server.entryURL }, [port2])
      await ready
    } catch (error) {
      if (this.destroyed) throw failure(this.terminalCode)
      const code = error instanceof Error && error.message === 'office-assets-unavailable' ? 'office-assets-unavailable' : 'office-startup-failed'
      this.fail(code)
      throw failure(code)
    }
  }

  private receive(value: unknown): void {
    if (this.destroyed || !officeRecord(value) || value.channel !== this.channel) return
    if (isOfficeEngineFailure(value)) { this.fail(this.ready ? 'office-engine-closed-unknown' : 'office-startup-failed'); return }
    if (!this.ready) {
      if (!isOfficeReady(value)) { this.fail('office-protocol-invalid'); return }
      this.ready = true
      if (this.readyTimer) clearTimeout(this.readyTimer)
      this.readyResolve?.(); this.readyResolve = this.readyReject = undefined
      return
    }
    if (value.type === 'result') {
      const pending = this.pending
      // Unrelated or late replies never settle a different operation.
      if (!pending || !sameOfficeEnvelope(value as unknown as OfficeResult, pending.request)) return
      if (!isOfficeResult(value)) { this.fail('office-protocol-invalid'); return }
      if (!value.ok) {
        clearTimeout(pending.timer); this.pending = undefined
        pending.reject(failure(value.error!))
        if (['engine-timeout-state-unknown', 'surface-state-unknown-recreate-required'].includes(value.error!)) this.fail('office-engine-closed-unknown')
        return
      }
      // Only a correlated persistence acknowledgement can change base revision.
      const expectedVersion = pending.request.command === 'ack' && pending.request.status === 'committed' ? pending.request.persistedVersion : pending.request.version
      if (value.state && (value.state.version !== expectedVersion || (!this.editing && pending.request.command !== 'edit' && (value.state.changeSequence !== 0 || value.state.acknowledgedSequence !== 0 || value.state.dirty)) || (this.current && (value.state.kind !== this.current.kind || value.state.changeSequence < this.current.changeSequence)))) { this.fail('office-protocol-invalid'); return }
      if (pending.request.command === 'open' && value.state!.kind !== pending.request.kind) { this.fail('office-protocol-invalid'); return }
      clearTimeout(pending.timer); this.pending = undefined
      if (value.command === 'edit') this.editing = true
      if (value.state) this.current = { ...value.state }
      if (value.command === 'open') this.openOperationId = value.operationId
      if (value.command === 'close') { this.current = undefined; this.openOperationId = undefined; this.editing = false }
      pending.resolve(value)
      return
    }
    if (!isOfficeEvent(value)) { this.fail('office-protocol-invalid'); return }
    if (!this.current || value.documentId !== this.current.documentId || value.version !== this.current.version || value.operationId !== this.openOperationId) return
    if (value.state) {
      if (value.state.kind !== this.current.kind || (!this.editing && (value.state.changeSequence !== 0 || value.state.acknowledgedSequence !== 0 || value.state.dirty)) || value.state.changeSequence < this.current.changeSequence) { this.fail('office-protocol-invalid'); return }
      this.current = { ...value.state }
    }
    if (value.selection && value.selection.changeSequence !== this.current.changeSequence) return
    this.emit(value)
  }

  private emit(event: OfficeEvent | OfficeSurfaceFatal): void {
    try { this.options.onEvent(structuredClone(event)) } catch { /* Host callback failures never expose content through diagnostics. */ }
  }
  private fail(code: OfficeSurfaceFatal['code']): void {
    if (this.destroyed) return
    this.emit({ type: 'fatal', code })
    this.dispose(code)
  }
  destroy(): void { this.dispose('office-surface-destroyed') }
  private dispose(code: string): void {
    if (this.destroyed) return
    this.destroyed = true; this.ready = false; this.terminalCode = code
    this.options.owner.removeListener('closed', this.ownerClosed)
    if (this.readyTimer) clearTimeout(this.readyTimer)
    this.readyReject?.(failure(code)); this.readyResolve = this.readyReject = undefined
    if (this.pending) { clearTimeout(this.pending.timer); this.pending.reject(failure(code)); this.pending = undefined }
    this.port?.close(); this.port = undefined
    if (this.attached && !this.options.owner.isDestroyed() && this.view) this.options.owner.contentView.removeChildView(this.view)
    this.attached = false
    if (this.view && !this.view.webContents.isDestroyed()) this.view.webContents.close({ waitForBeforeUnload: false })
    this.view = undefined
    this.server?.destroy(); this.server = undefined
    void this.officeSession?.closeAllConnections().catch(() => {})
    void this.officeSession?.clearStorageData().catch(() => {})
    this.current = undefined; this.openOperationId = undefined; this.seen.clear()
  }
}
