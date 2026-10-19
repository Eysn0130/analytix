import { app, MessageChannelMain, session, WebContentsView, type BrowserWindow, type Input, type MessagePortMain, type Rectangle, type Session } from 'electron'
import { logNativeOfficeFailure, type NativeOfficeDiagnosticErrorCode, type NativeOfficeDiagnosticStage } from '../logger'
import type { NativeOfficeActionMenu, NativeOfficeAppearance, NativeOfficeFormatting, NativeOfficeFormatChoice } from '../../shared/native-office'
import { nativeWorkspaceCommandFromInput } from '../../shared/native-office'
import { randomUUID } from 'node:crypto'
import { lstat, realpath } from 'node:fs/promises'
import { isAbsolute, resolve } from 'node:path'
import { isOfficePrivateAdmission, loadAdmittedOfficeBuffers, type OfficePrivateAdmission } from './office-private-admission'
import { loadOfficeBuffers, startOfficeAssetServer, type OfficeAssetServer } from './office-assets'
import { OFFICE_BIND_IPC, isOfficePointerMessage, isOfficeFormatting, isOfficeRangeLease, isOfficeFormatState, type OfficePointerGuest, type OfficePointerLease, isOfficeEvent, isOfficeEngineFailure, isOfficeReady, isOfficeRequest, isOfficeResult, officeRecord, sameOfficeEnvelope, type OfficeEvent, type OfficeRequest, type OfficeRequestInput, type OfficeResult, type OfficeState } from './office-protocol'

export type OfficeSurfaceFatal = { type: 'fatal'; code: 'office-startup-failed' | 'office-assets-unavailable' | 'office-protocol-invalid' | 'office-operation-timeout-unknown' | 'office-engine-closed-unknown' }
export type NativeOfficeSurfaceOptions = {
  owner: BrowserWindow
  protectedPresent?: boolean
  onEvent: (event: OfficeEvent | OfficeSurfaceFatal) => void
} & ({ assetRoot:string; sourceRoot:string; preloadPath:string; privateAdmission?:never } | { privateAdmission:OfficePrivateAdmission; assetRoot?:never; sourceRoot?:never; preloadPath?:never })
type Pending = { request: OfficeRequest; resolve: (result: OfficeResult) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout> }
const failure = (code: string) => new Error(code)
const validBounds = (bounds: Rectangle) => ['x', 'y', 'width', 'height'].every(k => Number.isSafeInteger(bounds[k as keyof Rectangle]) && bounds[k as keyof Rectangle] >= 0 && bounds[k as keyof Rectangle] <= 16384) && bounds.width > 0 && bounds.height > 0

// Main-owned source-experiment surface. The caller retains Core document authority.
export class NativeOfficeSurface {
  private diagnosticStage: NativeOfficeDiagnosticStage = 'surface_admission'
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
  private nativeWritable = false
  private visible = false
  private selectionToken?: string
  private selectionFormatting?: Pick<NativeOfficeFormatting,'rangeLease'|'formatState'>
  private pointerSelection?: Record<string,unknown>
  private selectionManual?: OfficeState['manual']
  private menuControlDown = false
  private menuMouseButton?: 'left' | 'right'
  private menuInputObserved = false
  private canRequestMenu(): boolean {
    return !this.options.protectedPresent && !this.destroyed && this.ready && this.visible && this.attached &&
      !!this.current && !this.pending && !this.options.owner.isDestroyed()
  }
  private readonly observeMenuInput = (_event: unknown, input: Input): void => {
    if (input.type === 'keyDown' || input.type === 'keyUp') this.menuControlDown = !this.destroyed && this.ready && this.visible &&
      this.attached && !this.options.owner.isDestroyed() && input.control === true
  }
  private readonly clearMenuInput = (): void => { this.menuControlDown = false; this.menuMouseButton = undefined }
  private acceptPointerSelection(selection:Record<string,unknown>): void {
    const token = typeof selection.token === 'string' ? selection.token : undefined
    const sample = selection.kind === 'text' && isOfficeRangeLease(selection.rangeLease) && isOfficeFormatState(selection.formatState) &&
      selection.rangeLease.token === token && selection.rangeLease.sequence === selection.changeSequence
      ? {rangeLease:structuredClone(selection.rangeLease),formatState:structuredClone(selection.formatState)} : undefined
    const previous = this.pointerSelection, previousSample = this.selectionFormatting, manual = this.current?.manual
    const sameSample = sample || previousSample ? !!sample && !!previousSample &&
      (['generation','sequence','token'] as const).every(key => sample.rangeLease[key] === previousSample.rangeLease[key]) &&
      (['fontFamily','fontSize','bold','italic','underline'] as const).every(key => sample.formatState[key] === previousSample.formatState[key]) : true
    const sameManual = manual || this.selectionManual ? !!manual && !!this.selectionManual && this.editing && this.nativeWritable && !manual.frozen &&
      (['version','origin','changeId','baseRevision','generation','frozen'] as const).every(key => manual[key] === this.selectionManual![key]) &&
      (!sample || sample.rangeLease.generation === manual.generation) : true
    // Duplicate native samples can arrive while validating an already offered
    // range. Keep that exact lease; real selection or authority changes retire it.
    const duplicate = !!this.pointer && !this.options.owner.isDestroyed() && this.isPointerCurrent(this.pointer.lease) && selection.kind === 'text' && previous?.kind === 'text' &&
      typeof selection.text === 'string' && !!selection.text.trim() && token === this.pointer.lease.selectionToken &&
      ['documentId','version','changeSequence','token','text','scope'].every(key => selection[key] === previous[key]) &&
      selection.documentId === this.current?.documentId && selection.version === this.current?.version &&
      selection.changeSequence === this.current?.changeSequence && sameSample && sameManual
    if (!duplicate) this.dismissPointerActions()
    this.selectionToken = token; this.pointerSelection = structuredClone(selection)
    this.selectionManual = manual ? {...manual} : undefined
    this.selectionFormatting = sample
  }
  private isFormattingCurrent(formatting:NativeOfficeFormatting): boolean {
    const manual = this.current?.manual, sample = this.selectionFormatting
    return isOfficeFormatting(formatting) && this.editing && this.nativeWritable && !!manual && !manual.frozen &&
      manual.changeId === formatting.changeId && manual.generation === formatting.rangeLease.generation &&
      this.current?.changeSequence === formatting.rangeLease.sequence && this.selectionToken === formatting.rangeLease.token && !!sample &&
      (['generation','sequence','token'] as const).every(key => sample.rangeLease[key] === formatting.rangeLease[key]) &&
      (['fontFamily','fontSize','bold','italic','underline'] as const).every(key => sample.formatState[key] === formatting.formatState[key])
  }
  private pointerView?: string
  private acceptPointerView(view:unknown): void {
    const next = JSON.stringify(view)
    if (this.pointerView !== undefined && this.pointerView !== next || !view) this.dismissPointerActions()
    this.pointerView = next
  }
  private pointer?: {lease:OfficePointerLease; actions?:Set<string>; formatting?:NativeOfficeFormatting; resolve?:(choice:string|null|NativeOfficeFormatChoice)=>void}

  private matchesPointer(lease:Pick<OfficePointerLease,'interactionId'|'selectionToken'>): boolean {
    return !!this.pointer && this.pointer.lease.interactionId === lease.interactionId && this.pointer.lease.selectionToken === lease.selectionToken
  }
  isPointerCurrent(lease:Pick<OfficePointerLease,'interactionId'|'selectionToken'>): boolean {
    return this.matchesPointer(lease) && this.visible && this.attached && !this.destroyed && !this.options.protectedPresent &&
      this.current?.kind === 'docx' && this.current.documentId === this.pointer!.lease.documentId && this.current.version === this.pointer!.lease.version &&
      this.current.changeSequence === this.pointer!.lease.expectedChangeSequence && this.selectionToken === lease.selectionToken
  }
  dismissPointerActions(lease?:Pick<OfficePointerLease,'interactionId'|'selectionToken'>): void {
    if (!this.pointer || lease && !this.matchesPointer(lease)) return
    const pending = this.pointer; this.pointer = undefined
    pending.resolve?.(null)
    try { this.port?.postMessage({...pending.lease,type:'pointer-actions',phase:'dismiss'}) } catch { /* Teardown also retires the lease. */ }
  }
  presentPointerActions(target:NativeOfficeActionMenu): Promise<string|null|NativeOfficeFormatChoice> {
    if (!target.pointer || !this.isPointerCurrent(target.pointer) || this.pointer!.actions || target.objectId !== this.current?.documentId ||
      target.revision !== this.current?.version || target.expectedChangeSequence !== this.current?.changeSequence ||
      target.formatting && !this.isFormattingCurrent(target.formatting)) return Promise.resolve(null)
    const pending = this.pointer!
    pending.actions = new Set(target.actions.filter(a => a.enabled).map(a => a.id))
    pending.formatting = target.formatting ? structuredClone(target.formatting) : undefined
    return new Promise(resolve => {
      pending.resolve = resolve
      try { this.port!.postMessage({...pending.lease,type:'pointer-actions',phase:'present',actions:target.actions,...(pending.formatting ? {formatting:pending.formatting} : {})}) }
      catch { this.dismissPointerActions() }
    })
  }
  private receivePointer(message:OfficePointerGuest): void {
    if (message.phase === 'request') {
      this.dismissPointerActions()
      if (!this.visible || !this.attached || this.options.protectedPresent || this.current?.kind !== 'docx' ||
        message.documentId !== this.current.documentId || message.version !== this.current.version || message.operationId !== this.openOperationId ||
        message.expectedChangeSequence !== this.current.changeSequence || message.selectionToken !== this.selectionToken || this.options.owner.isDestroyed()) return
      const lease:OfficePointerLease = {channel:message.channel,operationId:message.operationId,documentId:message.documentId,version:message.version,
        expectedChangeSequence:message.expectedChangeSequence,interactionId:message.interactionId,selectionToken:message.selectionToken}
      this.pointer = {lease}
      this.options.owner.webContents.send('office:menu-requested',{objectId:lease.documentId,revision:lease.version,expectedChangeSequence:lease.expectedChangeSequence,
        pointer:{interactionId:lease.interactionId,selectionToken:lease.selectionToken}})
    } else if (this.matchesPointer(message)) {
      if (message.phase === 'cancel') this.dismissPointerActions(message)
      else if (message.phase === 'choice' && this.isPointerCurrent(message) && this.pointer?.resolve && this.pointer.actions?.has(message.actionId)) {
        const resolve = this.pointer.resolve; this.pointer.resolve = undefined
        resolve(message.actionId)
      } else if (message.phase === 'format' && this.isPointerCurrent(message) && this.pointer?.resolve && this.pointer.formatting &&
        this.isFormattingCurrent(this.pointer.formatting) && message.changeId === this.pointer.formatting.changeId &&
        (['generation','sequence','token'] as const).every(key => message.rangeLease[key] === this.pointer!.formatting!.rangeLease[key])) {
        const resolve = this.pointer.resolve; this.pointer.resolve = undefined
        resolve({actionId:null,changeId:message.changeId,rangeLease:structuredClone(message.rangeLease),format:structuredClone(message.format)})
      } else this.dismissPointerActions(message)
    }
  }
  private pending?: Pending
  private seen = new Set<string>()
  private readyResolve?: () => void
  private readyReject?: (error: Error) => void
  private readyTimer?: ReturnType<typeof setTimeout>
  private readonly ownerClosed = () => this.destroy()

  constructor(private readonly options: NativeOfficeSurfaceOptions) {
    if (app.isPackaged ? !isOfficePrivateAdmission(options.privateAdmission) : options.privateAdmission !== undefined) throw failure('office-source-experiment-only')
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
    if (epoch === this.visibilityEpoch && !this.destroyed) { this.visible = true; this.view!.setVisible(true) }
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

  hide(): void { this.visible = false; this.clearMenuInput(); this.dismissPointerActions(); this.visibilityEpoch++; if (!this.destroyed) this.view?.setVisible(false) }

  async request(input: OfficeRequestInput): Promise<OfficeResult> {
    if (!officeRecord(input) || Object.hasOwn(input, 'channel')) throw failure('office-invalid-request')
    const request = { ...input, channel: this.channel }
    if (!isOfficeRequest(request)) throw failure('office-invalid-request')
    if (this.options.protectedPresent && !['open','local-view','close'].includes(request.command)) throw failure('office-invalid-request')
    if (!this.editing && !['local-view','open','edit','manual-edit','captureSelection','captureCells','validateSelection','close'].includes(request.command)) throw failure('office-invalid-request')
    if (!['captureSelection','captureCells','validateSelection'].includes(request.command) && !(request.command === 'local-view' && ['read','play-read'].includes(request.action ?? ''))) this.dismissPointerActions()
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
    this.diagnosticStage = request.command === 'open' ? 'surface_open' : 'surface_request'
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
      this.diagnosticStage = 'surface_admission'
      const admitted = this.options.privateAdmission ? await loadAdmittedOfficeBuffers(this.options.privateAdmission) : undefined
      this.diagnosticStage = 'surface_preload'
      const preloadPath = admitted?.preloadPath ?? this.options.preloadPath!
      if (!isAbsolute(preloadPath) || resolve(preloadPath) !== preloadPath || await realpath(preloadPath) !== preloadPath) throw failure('office-assets-unavailable')
      const preloadStat = await lstat(preloadPath)
      if (!preloadStat.isFile() || preloadStat.isSymbolicLink() || preloadStat.nlink !== 1) throw failure('office-assets-unavailable')
      this.diagnosticStage = 'surface_assets'
      const buffers = admitted?.buffers ?? await loadOfficeBuffers(this.options.sourceRoot!, this.options.assetRoot!)
      if (this.destroyed) { buffers.clear(); throw failure('office-surface-destroyed') }
      this.diagnosticStage = 'surface_asset_server'
      const server = await startOfficeAssetServer(buffers, randomUUID())
      if (this.destroyed) { server.destroy(); throw failure('office-surface-destroyed') }
      this.server = server
      this.diagnosticStage = 'surface_view'
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
      if (this.options.protectedPresent) contents.setIgnoreMenuShortcuts(true)
      const requestMenu = () => {
        if (!this.canRequestMenu()) return
        this.dismissPointerActions()
        this.options.owner.webContents.send('office:menu-requested', {
          objectId:this.current!.documentId, revision:this.current!.version, expectedChangeSequence:this.current!.changeSequence
        })
      }
      // Electron 41's mouse converter omits modifiers. Observe the two owned
      // keyboard streams so macOS Control-click also works across internal focus.
      this.options.owner.webContents.on('before-input-event', this.observeMenuInput)
      this.options.owner.on('blur', this.clearMenuInput)
      this.menuInputObserved = true
      contents.on('before-mouse-event', (event, mouse) => {
        const contextButton = mouse.button === 'right' || process.platform === 'darwin' && mouse.button === 'left' &&
          (this.menuControlDown || mouse.modifiers?.some(modifier => modifier === 'control' || modifier === 'ctrl'))
        if (mouse.type === 'contextMenu') { event.preventDefault(); return }
        if (mouse.type === 'mouseUp') {
          if (contextButton || mouse.button === this.menuMouseButton) {
            event.preventDefault()
            if (mouse.button === this.menuMouseButton) this.menuMouseButton = undefined
          }
          return
        }
        if (mouse.type !== 'mouseDown') return
        if (!contextButton) { this.menuMouseButton = undefined; return }
        // Keep these events out of Qt, including a left release after Control
        // is released first. Only the press emits; subsequent context events do not.
        event.preventDefault()
        this.menuMouseButton = mouse.button as 'left' | 'right'
        requestMenu()
      })
      contents.on('before-input-event', (event, input) => {
        this.observeMenuInput(event, input)
        if (this.options.protectedPresent && (input.control || input.meta) && ['c','x'].includes(input.key.toLowerCase())) { event.preventDefault(); return }
        if (input.type !== 'keyDown' || input.isAutoRepeat || !this.current) return
        if (!input.isComposing && !input.control && !input.meta && !input.alt &&
          (input.key === 'ContextMenu' || input.key === 'F10' && input.shift)) {
          event.preventDefault(); requestMenu(); return
        }
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
      this.diagnosticStage = 'surface_load'
      await contents.loadURL(server.entryURL)
      if (this.destroyed) throw failure(this.terminalCode)
      if (this.options.protectedPresent) {
        // Installed before binding/ready and before the first document bytes.
        this.diagnosticStage = 'surface_transfer_guard'
        await contents.executeJavaScriptInIsolatedWorld(1004,[{code:`(() => {
          const apply = Reflect.apply, prevent = Event.prototype.preventDefault, stop = Event.prototype.stopImmediatePropagation;
          const clear = DataTransfer.prototype.clearData;
          for (const type of ['copy','cut','dragstart']) window.addEventListener(type,event => {
            const transfer = event.clipboardData || event.dataTransfer;
            if (transfer) { try { apply(clear,transfer,[]); } catch {} }
            apply(prevent,event,[]); apply(stop,event,[]);
          },true);
        })();`}])
        if (this.destroyed) throw failure(this.terminalCode)
      }
      this.diagnosticStage = 'surface_bind'
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
      this.diagnosticStage = 'surface_ready'
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
    if (isOfficeEngineFailure(value)) { this.fail(this.ready ? 'office-engine-closed-unknown' : 'office-startup-failed', value.error); return }
    if (!this.ready) {
      if (!isOfficeReady(value)) { this.fail('office-protocol-invalid'); return }
      this.ready = true
      this.diagnosticStage = 'surface_ready'
      if (this.readyTimer) clearTimeout(this.readyTimer)
      this.readyResolve?.(); this.readyResolve = this.readyReject = undefined
      return
    }
    if (isOfficePointerMessage(value,'guest')) { this.receivePointer(value as OfficePointerGuest); return }
    if (value.type === 'result') {
      const pending = this.pending
      // Unrelated or late replies never settle a different operation.
      if (!pending || !sameOfficeEnvelope(value as unknown as OfficeResult, pending.request)) return
      if (!isOfficeResult(value)) { this.fail('office-protocol-invalid'); return }
      if (!value.ok) {
        clearTimeout(pending.timer); this.pending = undefined
        pending.reject(failure(value.error!))
        if (['engine-timeout-state-unknown', 'surface-state-unknown-recreate-required'].includes(value.error!)) this.fail('office-engine-closed-unknown')
        else this.diagnosticStage = 'surface_ready'
        return
      }
      // Only a correlated persistence acknowledgement can change base revision.
      const expectedVersion = pending.request.command === 'ack' && pending.request.status === 'committed' ? pending.request.persistedVersion : pending.request.version
      if (value.state && (value.state.version !== expectedVersion || (!this.editing && !['edit','manual-edit'].includes(pending.request.command) && (value.state.dirty || !this.nativeWritable && (value.state.changeSequence !== 0 || value.state.acknowledgedSequence !== 0))) || (this.current && (value.state.kind !== this.current.kind || value.state.changeSequence < this.current.changeSequence)))) { this.fail('office-protocol-invalid'); return }
      const nextManual = value.state?.manual, currentManual = this.current?.manual
      if (pending.request.command === 'ack' && pending.request.status === 'committed' &&
        (value.state?.acknowledgedSequence !== pending.request.exportedSequence || currentManual &&
          (value.state?.changeSequence !== pending.request.exportedSequence || value.state?.dirty))) { this.fail('office-protocol-invalid'); return }
      if (pending.request.command === 'manual-edit') {
        const authority = pending.request.manual
        if (!authority || !nextManual || nextManual.changeId !== authority.changeId || nextManual.baseRevision !== authority.baseRevision || nextManual.frozen || value.state?.kind !== 'docx') { this.fail('office-protocol-invalid'); return }
      } else if (currentManual) {
        const ending = pending.request.command === 'ack' && pending.request.status === 'committed'
        if (ending ? !!nextManual : !nextManual || nextManual.changeId !== currentManual.changeId || nextManual.baseRevision !== currentManual.baseRevision || nextManual.generation !== currentManual.generation) { this.fail('office-protocol-invalid'); return }
      } else if (nextManual) { this.fail('office-protocol-invalid'); return }
      if (value.view && ((!this.editing && value.view.modified) || !value.view.readOnly && (!this.nativeWritable || value.state?.kind !== 'docx' || !!this.options.protectedPresent))) { this.fail('office-protocol-invalid'); return }
      if (pending.request.command === 'open' && value.state!.kind !== pending.request.kind) { this.fail('office-protocol-invalid'); return }
      clearTimeout(pending.timer); this.pending = undefined
      if (value.command === 'edit') this.editing = true
      if (value.command === 'manual-edit') { this.editing = true; this.nativeWritable = true }
      if (value.command === 'ack' && pending.request.status === 'committed' && currentManual) this.editing = false
      if (value.state) this.current = { ...value.state }
      if (value.view) this.acceptPointerView(value.view)
      if (value.selection) this.acceptPointerSelection(value.selection)
      if (value.command === 'open') this.openOperationId = value.operationId
      if (value.command === 'close') { this.current = undefined; this.openOperationId = undefined; this.editing = false; this.nativeWritable = false }
      this.diagnosticStage = 'surface_ready'
      pending.resolve(value)
      return
    }
    if (!isOfficeEvent(value)) { this.fail('office-protocol-invalid'); return }
    if (!this.current || value.documentId !== this.current.documentId || value.version !== this.current.version || value.operationId !== this.openOperationId) return
    if (value.state) {
      if (value.state.kind !== this.current.kind || (!this.editing && (value.state.dirty || !this.nativeWritable && (value.state.changeSequence !== 0 || value.state.acknowledgedSequence !== 0))) || value.state.changeSequence < this.current.changeSequence) { this.fail('office-protocol-invalid'); return }
      const before = this.current.manual, after = value.state.manual
      if (before ? !after || before.changeId !== after.changeId || before.baseRevision !== after.baseRevision || before.generation !== after.generation : !!after) { this.fail('office-protocol-invalid'); return }
      this.current = { ...value.state }
    }
    if (value.type === 'view' && value.view && ((!this.editing && value.view.modified) || !value.view.readOnly && (!this.nativeWritable || this.current.kind !== 'docx' || !!this.options.protectedPresent) ||
      (this.current.kind === 'pptx' ? value.view.page === null : value.view.page !== null))) { this.fail('office-protocol-invalid'); return }
    if (value.selection && value.selection.changeSequence !== this.current.changeSequence) return
    if (value.type === 'cell-edit-requested' && (this.current.kind !== 'xlsx' || this.current.dirty || this.options.protectedPresent)) return
    if (value.type === 'view') this.acceptPointerView(value.view)
    else if (['selection','cell-edit-requested'].includes(value.type) && value.selection) this.acceptPointerSelection(value.selection)
    else if (value.type !== 'save-requested') this.dismissPointerActions()
    this.emit(value)
  }

  private emit(event: OfficeEvent | OfficeSurfaceFatal): void {
    try { this.options.onEvent(structuredClone(event)) } catch { /* Host callback failures never expose content through diagnostics. */ }
  }
  private fail(code: OfficeSurfaceFatal['code'], diagnosticCode: NativeOfficeDiagnosticErrorCode = code): void {
    if (this.destroyed) return
    logNativeOfficeFailure(this.diagnosticStage, diagnosticCode, this.options.protectedPresent === true)
    this.emit({ type: 'fatal', code })
    this.dispose(code)
  }
  destroy(): void { this.dispose('office-surface-destroyed') }
  private dispose(code: string): void {
    if (this.destroyed) return
    this.visible = false; this.dismissPointerActions(); this.selectionToken = undefined; this.selectionFormatting = undefined; this.pointerSelection = undefined; this.selectionManual = undefined
    this.clearMenuInput()
    if (this.menuInputObserved) {
      this.options.owner.webContents.removeListener('before-input-event', this.observeMenuInput)
      this.options.owner.removeListener('blur', this.clearMenuInput)
      this.menuInputObserved = false
    }
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
