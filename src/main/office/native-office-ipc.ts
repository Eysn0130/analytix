import { app, dialog, ipcMain, Menu, type BrowserWindow } from 'electron'
import { extname, isAbsolute, join } from 'node:path'
import { realpathSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { nativeOfficeActionMenuSchema, nativeOfficePickerRequestSchema, nativeOfficeRequestSchema, nativeOfficeInputFreezeSchema } from '../../shared/native-office'
import { createNativeOfficeController } from './native-office-controller'
import { isOfficePrivateAdmission, type OfficePrivateAdmission } from './office-private-admission'
import { NativeOfficeSurface } from './native-office-surface'
import type { PluginPackageHostRequest, PluginPackageHostResponse } from '../../../packages/runtime/src/contracts/plugin-package-host'

let quitGuard: {prepare:() => Promise<boolean>; cancel:() => Promise<void>} | undefined
/** Must run while the existing Core and its native sessions are still alive. */
export async function prepareNativeOfficeQuit(): Promise<boolean> {
  try { return await quitGuard?.prepare() ?? true } catch { await cancelNativeOfficeQuit(); return false }
}
export async function cancelNativeOfficeQuit(): Promise<void> { await quitGuard?.cancel() }

/** The Office view never receives this IPC or the generic desktop bridge. */
export function registerNativeOfficeIpc(
  getMainWindow: () => BrowserWindow | null,
  packageHost: (request: PluginPackageHostRequest) => Promise<PluginPackageHostResponse>,
  privateAdmission?: () => Promise<OfficePrivateAdmission>
): void {
  const admit = async () => {
    if (!privateAdmission) throw Error('office-assets-unavailable')
    const result = await privateAdmission()
    if (!isOfficePrivateAdmission(result)) throw Error('office-assets-unavailable')
    return result
  }
  let owner: BrowserWindow | undefined
  let controllerPrivate = false
  let controller: ReturnType<typeof createNativeOfficeController> | undefined
  const freezes = new Map<string, {main:BrowserWindow; frame:Electron.WebFrameMain; frozen:boolean; finish:(ok:boolean)=>void}>()
  const freezeInput = (main:BrowserWindow, frozen:boolean) => new Promise<boolean>(resolve => {
    const requestId = randomUUID(), frame = main.webContents.mainFrame
    const finish = (ok:boolean) => { clearTimeout(timer); freezes.delete(requestId); resolve(ok) }
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
      event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame) return cancelled
    const frame = main.webContents.mainFrame, target = parsed.data
    const current = () => {
      const view = controller?.getView()
      return getMainWindow() === main && !main.isDestroyed() && main.webContents.mainFrame === frame &&
        view?.objectId === target.objectId && view.revision === target.revision && (view.changeSequence ?? 0) === target.expectedChangeSequence
    }
    if (!current()) return cancelled
    return await new Promise<{actionId:string | null}>(resolve => {
      const menu = Menu.buildFromTemplate(target.actions.map(action => ({label:action.label,enabled:action.enabled,
        click:() => resolve(action.enabled && current() ? {actionId:action.id} : cancelled)})))
      menu.popup({window:main,callback:() => resolve(cancelled)})
    })
  })
  ipcMain.handle('office:pick-file', async (event, payload: unknown) => {
    const main = getMainWindow()
    const unavailable = { ok: false as const, error: 'unavailable' as const }
    if (!main || main.isDestroyed() || event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame) return unavailable
    const parsed = nativeOfficePickerRequestSchema.safeParse(payload)
    if (!parsed.success || !isAbsolute(parsed.data.workspace)) return { ok: false, error: 'invalid_request' }
    const frame = main.webContents.mainFrame
    try {
      if (app.isPackaged) await admit()
      if (getMainWindow() !== main || main.isDestroyed() || main.webContents.mainFrame !== frame) return unavailable
      const result = await dialog.showOpenDialog(main, { title: app.getLocale().startsWith('zh') ? '打开办公文件' : 'Open Office file',
        defaultPath: parsed.data.workspace, filters: [{ name: parsed.data.kind.toUpperCase(), extensions: [parsed.data.kind] }],
        properties: ['openFile', 'dontAddToRecent'] })
      if (getMainWindow() !== main || main.isDestroyed() || main.webContents.mainFrame !== frame) return unavailable
      if (result.canceled) return { ok: true, path: null }
      if (result.filePaths.length !== 1 || !isAbsolute(result.filePaths[0]) || extname(result.filePaths[0]).toLowerCase() !== '.' + parsed.data.kind) return unavailable
      return { ok: true, path: result.filePaths[0] }
    } catch { return unavailable }
  })
  ipcMain.handle('office:request', async (event, payload: unknown) => {
    const main = getMainWindow()
    const unavailable = { ok: false as const, view: null, error: 'unavailable' as const }
    if (!main || main.isDestroyed() || event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame) return unavailable
    const incomingFrame = main.webContents.mainFrame
    const annotation = nativeOfficeRequestSchema.safeParse(payload)
    const recordingAnnotation = controllerPrivate && annotation.success && annotation.data.action === 'annotationSave'
    // Earlier Renderer note invokes must reach Main synchronously before its
    // freeze ACK. Core still verifies every annotation write and its CAS.
    if (app.isPackaged && !recordingAnnotation) {
      try { await admit() } catch { return unavailable }
      if (getMainWindow() !== main || main.isDestroyed() || main.webContents.mainFrame !== incomingFrame) return unavailable
    }
    if (!controller) {
      owner = main
      const sourceRoot = app.getAppPath()
      const assetRoot = app.isPackaged ? '' : process.env.ANALYTIX_DEVELOPMENT_OFFICE_ASSET_ROOT ?? ''
      controllerPrivate = app.isPackaged
      controller = createNativeOfficeController({ packageHost,
        freezeAnnotationInput: frozen => freezeInput(main,frozen),
        createSurface: async (onEvent) => app.isPackaged
          ? new NativeOfficeSurface({owner:main,privateAdmission:await admit(),onEvent})
          : new NativeOfficeSurface({ owner: main, sourceRoot, assetRoot,
            preloadPath: realpathSync(join(sourceRoot, 'out', 'preload', 'office.cjs')), onEvent }),
        onChange: (view) => { if (!main.isDestroyed()) main.webContents.send('office:changed', view) }
      })
      const current = controller
      let closing: Promise<boolean> | undefined, allowClose = false
      const cancel = async () => {
        allowClose = false
        await current.freezeAnnotationInput(false)
        await current.restoreVisibility()
      }
      const prepare = () => closing ??= (async () => {
        await current.freezeAnnotationInput(true)
        const annotations = await current.flushAnnotations()
        if (!annotations.ok || current.hasPendingAnnotations()) {
          await dialog.showMessageBox(main,{type:'warning',message:'标注备注尚未保存',detail:'备注已保留在当前窗口。请重试保存后退出。',buttons:['返回文档'],noLink:true})
          return false
        }
        if (current.getViews().some(view => view.dirty || view.saving)) {
          await current.request({action:'hide'})
          const pending = current.getViews().some(view => view.saving)
          const decision = await dialog.showMessageBox(main,{type:'warning',message:pending ? '文档更新结果尚未确认' : '文档更新尚未完成',
            detail:pending ? '先查询本次更新结果，再退出。' : '重试更新后退出，或放弃未保存的工作副本。',
            buttons:pending ? ['查询后退出','取消'] : ['重试后退出','放弃工作副本并退出','取消'],defaultId:pending ? 1 : 2,cancelId:pending ? 1 : 2,noLink:true})
          if (decision.response === 0) {
            const saved = await current.saveAll()
            if (!saved.ok || current.getViews().some(view => view.dirty || view.saving)) return false
          } else if (pending || decision.response !== 1) return false
        }
        allowClose = true
        return true
      })().catch(() => false).then(async accepted => {
        if (!accepted) await cancel()
        return accepted
      }).finally(() => { closing = undefined })
      const guard = {prepare,cancel}; quitGuard = guard
      main.on('close', (event) => {
        // The window's earlier tray/ask policy may have cancelled destruction.
        if (event.defaultPrevented) return
        if (allowClose || current.getViews().length === 0 && !current.hasPendingAnnotations()) return
        event.preventDefault()
        if (closing) return
        void prepare().then(accepted => { if (accepted) main.close() }).catch(() => undefined)
      })
      main.once('closed', () => {
        if (quitGuard === guard) quitGuard = undefined
        for (const pending of freezes.values()) if (pending.main === main) pending.finish(false)
        void current.destroy()
        if (controller === current) { controller = undefined; controllerPrivate = false; owner = undefined }
      })
    }
    if (owner !== main) return unavailable
    const frame = main.webContents.mainFrame
    let result = await controller.request(payload)
    const intent = nativeOfficeRequestSchema.safeParse(payload)
    if (!result.ok && result.error === 'unsaved_changes' && intent.success && intent.data.action === 'close') {
      const targetId = intent.data.objectId ?? result.view?.objectId
      const target = controller.getViews().find(view => view.objectId === targetId)
      if (target && !target.saving) {
        await controller.request({action:'hide'})
        try {
          const decision = await dialog.showMessageBox(main, {type:'warning', message:'文档更新尚未完成',
            buttons:['重试后关闭','放弃工作副本并关闭','取消'], defaultId:2, cancelId:2, noLink:true})
          if (getMainWindow() !== main || main.isDestroyed() || main.webContents.mainFrame !== frame) return unavailable
          if (decision.response === 0) {
            const saved = await controller.request({action:'save',objectId:target.objectId,revision:target.revision,expectedChangeSequence:target.changeSequence ?? 0})
            result = saved.ok ? await controller.request({action:'close',objectId:target.objectId}) : saved
          } else if (decision.response === 1) result = await controller.request({action:'close',objectId:target.objectId,discard:true})
        } finally { await controller.restoreVisibility() }
      }
    }
    if (getMainWindow() !== main || main.isDestroyed() || main.webContents.mainFrame !== frame) return unavailable
    return result
  })
}
