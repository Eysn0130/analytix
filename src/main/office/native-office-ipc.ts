import { app, dialog, ipcMain, type BrowserWindow } from 'electron'
import { extname, isAbsolute, join } from 'node:path'
import { realpathSync } from 'node:fs'
import { nativeOfficePickerRequestSchema } from '../../shared/native-office'
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
      main.once('closed', () => {
        void current.destroy()
        if (controller === current) { controller = undefined; owner = undefined }
      })
    }
    if (owner !== main) return unavailable
    const frame = main.webContents.mainFrame
    const result = await controller.request(payload)
    if (getMainWindow() !== main || main.isDestroyed() || main.webContents.mainFrame !== frame) return unavailable
    return result
  })
}
