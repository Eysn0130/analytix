import { logNativeOfficeFailure, type NativeOfficeDiagnosticStage } from '../logger'
import { app, dialog, ipcMain, Menu, type BrowserWindow } from 'electron'
import { extname, isAbsolute, join } from 'node:path'
import { realpathSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { nativeOfficeActionMenuSchema, nativeOfficeActionChoiceSchema, nativeOfficePickerRequestSchema, nativeOfficeRequestSchema, nativeOfficeInputFreezeSchema } from '../../shared/native-office'
import { createDocxEditorController } from './docx-editor-controller'
import { docxEditorRequestSchema,docxEditorCaptureSchema } from '../../shared/docx-editor'
import { createNativeOfficeController } from './native-office-controller'
import { isOfficePrivateAdmission, type OfficePrivateAdmission } from './office-private-admission'
import { NativeOfficeSurface } from './native-office-surface'
import type { PresentOfficeSnapshot } from '../ipc/tool-result-local-display-ipc'
import { captureNativeRendererDocument } from '../ipc/native-renderer-document'
import type { PluginPackageHostRequest, PluginPackageHostResponse } from '../../../packages/runtime/src/contracts/plugin-package-host'

let docxQuitGuard: {window:BrowserWindow; prepare:()=>Promise<boolean>; cancel:()=>void} | undefined
let quitGuard: {window: BrowserWindow; prepare:() => Promise<boolean>; cancel:() => Promise<void>} | undefined
const nativeOfficeTeardowns = new Set<Promise<void>>()
let nativeOfficeTeardownFailed = false
/** Must run while the existing Core and its native sessions are still alive. */
export async function prepareNativeOfficeQuit(window?: BrowserWindow): Promise<boolean> {
  if (window && quitGuard?.window !== window && docxQuitGuard?.window !== window) return true
  try {
    if (docxQuitGuard && (!window || docxQuitGuard.window === window) && !await docxQuitGuard.prepare()) return false
    return await quitGuard?.prepare() ?? true } catch { await cancelNativeOfficeQuit(); return false }
}
export async function cancelNativeOfficeQuit(): Promise<void> { docxQuitGuard?.cancel(); await quitGuard?.cancel() }
/** Wait for cleanup attempts, not a successful close-object receipt. */
export async function waitForNativeOfficeQuitTeardown(): Promise<void> {
  while (nativeOfficeTeardowns.size > 0) {
    await Promise.allSettled([...nativeOfficeTeardowns])
  }
  const failed = nativeOfficeTeardownFailed
  nativeOfficeTeardownFailed = false
  if (failed) throw new Error('native Office teardown did not settle cleanly')
}

/** The Office view never receives this IPC or the generic desktop bridge. */
export function registerNativeOfficeIpc(
  getMainWindow: () => BrowserWindow | null,
  packageHost: (request: PluginPackageHostRequest) => Promise<PluginPackageHostResponse>,
  privateAdmission?: () => Promise<OfficePrivateAdmission>,
  resolvePresent?: (event: Electron.IpcMainInvokeEvent, selector: unknown) => Promise<PresentOfficeSnapshot | null>
): void {
  let docx:ReturnType<typeof createDocxEditorController> | undefined
  let docxWindow:BrowserWindow | undefined
  let docxDelivery:ReturnType<typeof captureNativeRendererDocument> | undefined
  const docxCaptures=new Map<string,{main:BrowserWindow;frame:Electron.WebFrameMain;finish:(value:ReturnType<typeof docxEditorCaptureSchema.parse>|null)=>void}>()
  ipcMain.handle('docx-editor:capture-result',(event,raw:unknown)=>{
    const parsed=docxEditorCaptureSchema.safeParse(raw)
    if (!parsed.success) return {ok:false}
    const pending=docxCaptures.get(parsed.data.requestId)
    if (!pending || pending.main!==getMainWindow() || pending.main.isDestroyed() || event.sender!==pending.main.webContents ||
      event.senderFrame!==pending.frame || pending.main.webContents.mainFrame!==pending.frame || !docxDelivery?.isCurrent()) return {ok:false}
    pending.finish(parsed.data);return {ok:true}
  })
  ipcMain.handle('docx-editor:request', async (event, raw:unknown) => {
    const main=getMainWindow(), parsed=docxEditorRequestSchema.safeParse(raw)
    const unavailable={ok:false,view:null,error:'unavailable'}
    if (!parsed.success || !main || main.isDestroyed() || event.sender!==main.webContents || event.senderFrame!==main.webContents.mainFrame) return unavailable
    const frame=main.webContents.mainFrame
    const current=()=>getMainWindow()===main && !main.isDestroyed() && !main.webContents.isDestroyed() && main.webContents.mainFrame===frame
    const lease=captureNativeRendererDocument(main.webContents,current)
    try {
      if (!docx) {
        docx=createDocxEditorController(packageHost,view=>new Promise(resolve=>{
          const requestId=randomUUID()
          const finish=(value:ReturnType<typeof docxEditorCaptureSchema.parse>|null)=>{clearTimeout(timer);docxCaptures.delete(requestId);resolve(value)}
          const timer=setTimeout(()=>finish(null),5000)
          if (!current() || !docxDelivery?.isCurrent()) {finish(null);return}
          docxCaptures.set(requestId,{main,frame,finish})
          main.webContents.send('docx-editor:capture',{requestId,editorId:view.editorId,version:view.version,revision:view.revision})
        })); docxWindow=main
        const owned=docx
        docxDelivery=captureNativeRendererDocument(main.webContents,current,()=>owned.freeze(true))
        const guard={window:main,cancel:()=>owned.freeze(false),prepare:async()=>{
          owned.freeze(true)
          if (!owned.hasUnsettled()) return true
          await dialog.showMessageBox(main,{type:'warning',message:'Word 文档尚有未完成的修改',
            detail:'请返回文档保存、查询本次保存结果或明确放弃工作副本，再退出。',buttons:['返回文档'],noLink:true})
          owned.freeze(false);return false
        }}
        docxQuitGuard=guard
        main.once('closed',()=>{
          docxDelivery?.release()
          for (const pending of docxCaptures.values()) if (pending.main===main) pending.finish(null)
          if (docxQuitGuard===guard) docxQuitGuard=undefined
          const teardown=owned.destroy();nativeOfficeTeardowns.add(teardown)
          void teardown.then(()=>nativeOfficeTeardowns.delete(teardown),()=>{nativeOfficeTeardownFailed=true;nativeOfficeTeardowns.delete(teardown)})
          if (docx===owned) {docx=undefined;docxWindow=undefined}
        })
      }
      if (docxWindow!==main || !docxDelivery?.isCurrent()) return unavailable
      const result=await docx.request(parsed.data)
      return lease.isCurrent() && docxDelivery.isCurrent()?result:unavailable
    } finally {lease.release()}
  })
  const admit = async () => {
    if (!privateAdmission) throw Error('office-assets-unavailable')
    const result = await privateAdmission()
    if (!isOfficePrivateAdmission(result)) throw Error('office-assets-unavailable')
    return result
  }
  let owner: BrowserWindow | undefined
  let controllerPrivate = false
  let controller: ReturnType<typeof createNativeOfficeController> | undefined
  let delivery: ReturnType<typeof captureNativeRendererDocument> | undefined
  const currentDocument = (main: BrowserWindow, frame: Electron.WebFrameMain) =>
    getMainWindow() === main && !main.isDestroyed() && !main.webContents.isDestroyed() && main.webContents.mainFrame === frame
  const hideRetained = () => { void controller?.request({ action: 'hide' }) }
  const bindDelivery = (main: BrowserWindow, frame: Electron.WebFrameMain) => {
    delivery?.release()
    delivery = captureNativeRendererDocument(main.webContents, () => currentDocument(main, frame), hideRetained)
  }
  const freezes = new Map<string, {main:BrowserWindow; frame:Electron.WebFrameMain; frozen:boolean; finish:(ok:boolean)=>void}>()
  const freezeInput = (main:BrowserWindow, frozen:boolean) => new Promise<boolean>(resolve => {
    // A revoked document cannot contribute more input. Main still flushes all
    // notes it already received; do not ask its replacement to acknowledge it.
    if (!delivery?.isCurrent()) { resolve(true); return }
    const requestId = randomUUID(), frame = main.webContents.mainFrame
    const lease = captureNativeRendererDocument(main.webContents, () => currentDocument(main, frame) && delivery?.isCurrent() === true, () => finish(false))
    const finish = (ok:boolean) => { clearTimeout(timer); lease.release(); freezes.delete(requestId); resolve(ok) }
    const timer = setTimeout(() => finish(false),5000)
    freezes.set(requestId,{main,frame,frozen,finish})
    if (main.isDestroyed() || getMainWindow() !== main) { finish(false); return }
    main.webContents.send('office:annotation-input-freeze',{requestId,frozen})
  })
  ipcMain.handle('office:annotation-input-frozen', async (event,payload:unknown) => {
    const parsed = nativeOfficeInputFreezeSchema.safeParse(payload)
    if (!parsed.success) return {ok:false}
    const pending = freezes.get(parsed.data.requestId)
    if (!pending || pending.frozen !== parsed.data.frozen || pending.main.isDestroyed() || getMainWindow() !== pending.main ||
      event.sender !== pending.main.webContents || event.senderFrame !== pending.frame || pending.main.webContents.mainFrame !== pending.frame) return {ok:false}
    pending.finish(true); return {ok:true}
  })
  ipcMain.handle('office:action-menu', async (event, payload: unknown) => {
    const main = getMainWindow(), parsed = nativeOfficeActionMenuSchema.safeParse(payload)
    const cancelled = {actionId:null}
    if (!parsed.success || !main || main.isDestroyed() || owner !== main ||
      event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame || !delivery?.isCurrent()) return cancelled
    const frame = main.webContents.mainFrame, target = parsed.data
    const current = () => {
      const view = controller?.getView()
      if (getMainWindow() !== main || main.isDestroyed() || main.webContents.mainFrame !== frame ||
        view?.objectId !== target.objectId || view.revision !== target.revision || (view.changeSequence ?? 0) !== target.expectedChangeSequence) return false
      const formatting = target.formatting
      if (!formatting) return true
      const manual = view.manual, selected = view.selection
      return view.kind === 'docx' && !view.protectedPresent && view.status === 'ready' && view.editing === true && !view.saving &&
        !!manual && manual.version === 2 && manual.origin === 'manual' && !manual.frozen && manual.changeId === formatting.changeId &&
        manual.generation === formatting.rangeLease.generation && selected?.kind === 'text' && selected.token === target.pointer?.selectionToken &&
        !!selected.rangeLease && !!selected.formatState &&
        (['generation','sequence','token'] as const).every(key => selected.rangeLease![key] === formatting.rangeLease[key]) &&
        (['fontFamily','fontSize','bold','italic','underline'] as const).every(key => selected.formatState![key] === formatting.formatState[key])
    }
    if (!current() || controller?.getView()?.protectedPresent && target.actions.some(action => !/^view-(?:previous-page|next-page|fit|zoom-out|zoom-in|zoom-(?:25|50|100|200)|read)$/.test(action.id))) return cancelled
    if (target.pointer) {
      // Guest drawing is UI only. Main owns the document lease and native endpoint checks.
      if (!controller || controller.getView()?.protectedPresent || controller.getView()?.kind !== 'docx') return cancelled
      // A stale range cancels this menu, not the retained document surface.
      // Renderer-document revocation still hides it through the lifetime lease.
      const lease = captureNativeRendererDocument(main.webContents,
        () => currentDocument(main, frame) && delivery?.isCurrent() === true, hideRetained)
      try {
        const action = await controller.pointerActions(target)
        if (!lease.isCurrent() || !delivery?.isCurrent() || !current()) return cancelled
        const choice = nativeOfficeActionChoiceSchema.safeParse(typeof action === 'string' || action === null ? {actionId:action} : action)
        if (!choice.success) return cancelled
        const chosen = choice.data
        if ('format' in chosen && (!target.formatting || chosen.changeId !== target.formatting.changeId ||
          !(['generation','sequence','token'] as const).every(key => chosen.rangeLease[key] === target.formatting!.rangeLease[key]))) return cancelled
        return chosen
      } finally { lease.release() }
    }
    return await new Promise<{actionId:string | null}>(resolve => {
      const finish = (result: {actionId: string | null}) => { lease.release(); resolve(result) }
      const lease = captureNativeRendererDocument(main.webContents, current, () => finish(cancelled))
      const menu = Menu.buildFromTemplate(target.actions.map(action => ({label:action.label,enabled:action.enabled,
        click:() => finish(action.enabled && lease.isCurrent() ? {actionId:action.id} : cancelled)})))
      menu.popup({window:main,callback:() => finish(cancelled)})
    })
  })
  ipcMain.handle('office:pick-file', async (event, payload: unknown) => {
    const main = getMainWindow()
    const unavailable = { ok: false as const, error: 'unavailable' as const }
    if (!main || main.isDestroyed() || event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame) return unavailable
    const parsed = nativeOfficePickerRequestSchema.safeParse(payload)
    if (!parsed.success || !isAbsolute(parsed.data.workspace)) return { ok: false, error: 'invalid_request' }
    const frame = main.webContents.mainFrame
    const lease = captureNativeRendererDocument(main.webContents, () => currentDocument(main, frame))
    try {
      if (app.isPackaged && parsed.data.kind !== 'docx') await admit()
      if (!lease.isCurrent()) return unavailable
      const result = await dialog.showOpenDialog(main, { title: app.getLocale().startsWith('zh') ? '打开办公文件' : 'Open Office file',
        defaultPath: parsed.data.workspace, filters: [{ name: parsed.data.kind.toUpperCase(), extensions: [parsed.data.kind] }],
        properties: ['openFile', 'dontAddToRecent'] })
      if (!lease.isCurrent()) return unavailable
      if (result.canceled) return { ok: true, path: null }
      if (result.filePaths.length !== 1 || !isAbsolute(result.filePaths[0]) || extname(result.filePaths[0]).toLowerCase() !== '.' + parsed.data.kind) return unavailable
      return { ok: true, path: result.filePaths[0] }
    } catch { return unavailable } finally { lease.release() }
  })
  ipcMain.handle('office:request', async (event, payload: unknown) => {
    const main = getMainWindow()
    const unavailable = { ok: false as const, view: null, error: 'unavailable' as const }
    if (!main || main.isDestroyed() || event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame) return unavailable
    const incomingFrame = main.webContents.mainFrame
    const annotation = nativeOfficeRequestSchema.safeParse(payload)
    // Only an explicit open may reattach a retained controller after navigation.
    // Its existing Core package/recovery checks must succeed before subscription.
    const reattaching = controller !== undefined && !delivery?.isCurrent()
    const creating = annotation.success && annotation.data.action === 'create'
    if (reattaching && !creating && (!annotation.success || !['open','openPresent'].includes(annotation.data.action))) return unavailable
    const lease = captureNativeRendererDocument(main.webContents, () => currentDocument(main, incomingFrame), hideRetained)
    const opening = annotation.success && ['open', 'openPresent'].includes(annotation.data.action)
    const protectedPresent = annotation.success && annotation.data.action === 'openPresent'
    let diagnosticStage: NativeOfficeDiagnosticStage = 'ipc_admission'
    try {
      const recordingAnnotation = controllerPrivate && annotation.success && annotation.data.action === 'annotationSave'
      // Earlier Renderer note invokes must reach Main synchronously before its
      // freeze ACK. Core still verifies every annotation write and its CAS.
      if (app.isPackaged && !recordingAnnotation && !creating) {
        try { await admit() } catch {
          if (opening) logNativeOfficeFailure(diagnosticStage, 'unavailable', protectedPresent)
          return unavailable
        }
        if (!lease.isCurrent()) return unavailable
      }
      if (!controller) {
        owner = main
        bindDelivery(main, incomingFrame)
        const sourceRoot = app.getAppPath()
        const assetRoot = app.isPackaged ? '' : process.env.ANALYTIX_DEVELOPMENT_OFFICE_ASSET_ROOT ?? ''
        controllerPrivate = app.isPackaged
        controller = createNativeOfficeController({ packageHost,
          freezeAnnotationInput: frozen => freezeInput(main,frozen),
          createSurface: async (onEvent, protectedPresent) => app.isPackaged
            ? new NativeOfficeSurface({owner:main,privateAdmission:await admit(),onEvent,protectedPresent})
            : new NativeOfficeSurface({ owner: main, sourceRoot, assetRoot,
              preloadPath: realpathSync(join(sourceRoot, 'out', 'preload', 'office.cjs')), onEvent, protectedPresent }),
          onChange: (view) => { if (controller === current && owner === main && !main.isDestroyed() && delivery?.isCurrent()) main.webContents.send('office:changed', view) }
        })
        const current = controller
        let closing: Promise<boolean> | undefined
        const cancel = async () => {
          await current.freezeAnnotationInput(false)
          if (delivery?.isCurrent()) await current.restoreVisibility()
        }
        const prepare = () => closing ??= (async () => {
          await current.freezeAnnotationInput(true)
          const annotations = await current.flushAnnotations()
          if (!annotations.ok || current.hasPendingAnnotations()) {
            await dialog.showMessageBox(main,{type:'warning',message:'标注备注尚未保存',detail:'备注已保留在当前窗口。请重试保存后退出。',buttons:['返回文档'],noLink:true})
            return false
          }
          // Manual input can still be queued before its first dirty event.
          // Reuse the existing explicit save/discard choice before teardown.
          if (current.getViews().some(view => view.manual || view.dirty || view.saving)) {
            await current.request({action:'hide'})
            const pending = current.getViews().some(view => view.saving)
            const decision = await dialog.showMessageBox(main,{type:'warning',message:pending ? '文档更新结果尚未确认' : '文档更新尚未完成',
              detail:pending ? '先查询本次更新结果，再退出。' : '重试更新后退出，或放弃未保存的工作副本。',
              buttons:pending ? ['查询后退出','取消'] : ['重试后退出','放弃工作副本并退出','取消'],defaultId:pending ? 1 : 2,cancelId:pending ? 1 : 2,noLink:true})
            if (decision.response === 0) {
              const saved = await current.saveAll()
              if (!saved.ok || current.getViews().some(view => view.dirty || view.saving)) return false
            } else if (pending || decision.response !== 1) return false
            else {
              for (const view of current.getViews()) {
                if (!view.manual) continue
                const discarded = await current.request({action:'close',objectId:view.objectId,discard:true})
                if (!discarded.ok) return false
              }
            }
          }
          return true
        })().catch(() => false).then(async accepted => {
          if (!accepted) await cancel()
          return accepted
        }).finally(() => { closing = undefined })
        const guard = {window: main, prepare, cancel}; quitGuard = guard
        main.once('closed', () => {
          if (controller === current) delivery?.release()
          if (quitGuard === guard) quitGuard = undefined
          for (const pending of freezes.values()) if (pending.main === main) pending.finish(false)
          const teardown = Promise.resolve().then(() => current.destroy())
          nativeOfficeTeardowns.add(teardown)
          void teardown.then(
            () => nativeOfficeTeardowns.delete(teardown),
            () => { nativeOfficeTeardownFailed = true; nativeOfficeTeardowns.delete(teardown) }
          )
          if (controller === current) { controller = undefined; controllerPrivate = false; owner = undefined }
        })
      }
      if (owner !== main) return unavailable
      let result
      if (annotation.success && annotation.data.action === 'openPresent') {
        diagnosticStage = 'ipc_resolve_present'
        const snapshot = await resolvePresent?.(event, annotation.data.protectedPresent)
        if (!snapshot) { logNativeOfficeFailure(diagnosticStage, 'unavailable', true); return unavailable }
        try {
          if (!lease.isCurrent()) return unavailable
          diagnosticStage = 'ipc_controller_open'
          result = await controller.openPresent(annotation.data, snapshot)
        } finally { snapshot.bytes.fill(0) }
      } else {
        diagnosticStage = 'ipc_controller_open'
        result = await controller.request(payload)
      }
      if (!lease.isCurrent()) return unavailable
      if (reattaching && !creating) {
        if (!result.ok) return { ok: false, view: null, error: result.error ?? 'unavailable', ...(result.creation ? {creation:result.creation} : {}) }
        bindDelivery(main, incomingFrame)
      }
      const intent = nativeOfficeRequestSchema.safeParse(payload)
      if (!result.ok && result.error === 'unsaved_changes' && intent.success && intent.data.action === 'close') {
        const targetId = intent.data.objectId ?? result.view?.objectId
        const target = controller.getViews().find(view => view.objectId === targetId)
        if (target && !target.saving) {
          await controller.request({action:'hide'})
          try {
            const decision = await dialog.showMessageBox(main, {type:'warning', message:'文档更新尚未完成',
              buttons:['重试后关闭','放弃工作副本并关闭','取消'], defaultId:2, cancelId:2, noLink:true})
            if (!lease.isCurrent()) return unavailable
            if (decision.response === 0) {
              const saved = await controller.request({action:'save',objectId:target.objectId,revision:target.revision,expectedChangeSequence:target.changeSequence ?? 0})
              if (!lease.isCurrent()) return unavailable
              result = saved.ok ? await controller.request({action:'close',objectId:target.objectId}) : saved
            } else if (decision.response === 1) result = await controller.request({action:'close',objectId:target.objectId,discard:true})
          } finally { if (lease.isCurrent()) await controller.restoreVisibility() }
        }
      }
      if (!lease.isCurrent()) return unavailable
      return result
    } catch (error) {
      if (opening) logNativeOfficeFailure(diagnosticStage, 'unexpected', protectedPresent)
      throw error
    } finally { lease.release() }
  })
}
