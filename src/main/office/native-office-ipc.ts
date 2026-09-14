import { app, dialog, ipcMain, Menu, type BrowserWindow } from 'electron'
import { extname, isAbsolute, join } from 'node:path'
import { realpathSync } from 'node:fs'
import { nativeOfficeActionMenuSchema, nativeOfficePickerRequestSchema, nativeOfficeRequestSchema } from '../../shared/native-office'
import { createNativeOfficeController } from './native-office-controller'
import { NativeOfficeSurface } from './native-office-surface'
import type { PluginPackageHostRequest, PluginPackageHostResponse } from '../../../packages/runtime/src/contracts/plugin-package-host'

/** The Office view never receives this IPC or the generic desktop bridge. */
export function registerNativeOfficeIpc(
  getMainWindow: () => BrowserWindow | null,
  packageHost: (request: PluginPackageHostRequest) => Promise<PluginPackageHostResponse>
): void {
  let owner: BrowserWindow | undefined
  let controller: ReturnType<typeof createNativeOfficeController> | undefined
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
    if (app.isPackaged || !main || main.isDestroyed() || event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame) return unavailable
    const parsed = nativeOfficePickerRequestSchema.safeParse(payload)
    if (!parsed.success || !isAbsolute(parsed.data.workspace)) return { ok: false, error: 'invalid_request' }
    const frame = main.webContents.mainFrame
    try {
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
    if (app.isPackaged || !main || main.isDestroyed() || event.sender !== main.webContents || event.senderFrame !== main.webContents.mainFrame) return unavailable
    if (!controller) {
      owner = main
      const sourceRoot = app.getAppPath()
      const assetRoot = process.env.ANALYTIX_DEVELOPMENT_OFFICE_ASSET_ROOT ?? ''
      controller = createNativeOfficeController({ packageHost,
        createSurface: (onEvent) => new NativeOfficeSurface({ owner: main, sourceRoot, assetRoot,
          preloadPath: realpathSync(join(sourceRoot, 'out', 'preload', 'office.cjs')), onEvent }),
        onChange: (view) => { if (!main.isDestroyed()) main.webContents.send('office:changed', view) }
      })
      const current = controller
      let closing = false, allowClose = false
      main.on('close', (event) => {
        if (allowClose || !current.getViews().some(view => view.dirty || view.saving)) return
        event.preventDefault()
        if (closing) return
        closing = true
        void (async () => {
          await current.request({action:'hide'})
          const pending = current.getViews().some(view => view.saving)
          const decision = await dialog.showMessageBox(main, {type:'warning', message:pending ? '文档更新结果尚未确认' : '文档更新尚未完成',
            detail:pending ? '先查询本次更新结果，再退出。' : '重试更新后退出，或放弃未保存的工作副本。',
            buttons:pending ? ['查询后退出','取消'] : ['重试后退出','放弃工作副本并退出','取消'], defaultId:pending ? 1 : 2, cancelId:pending ? 1 : 2, noLink:true})
          if (decision.response === 0) {
            const saved = await current.saveAll()
            if (!saved.ok || current.getViews().some(view => view.dirty || view.saving)) return
          } else if (pending || decision.response !== 1) return
          allowClose = true
          main.close()
        })().finally(() => { closing = false; if (!allowClose) void current.restoreVisibility() }).catch(() => undefined)
      })
      main.once('closed', () => {
        void current.destroy()
        if (controller === current) { controller = undefined; owner = undefined }
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
