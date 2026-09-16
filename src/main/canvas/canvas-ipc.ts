import { BrowserWindow, dialog, ipcMain } from 'electron'
import { extname, isAbsolute } from 'node:path'
import { z } from 'zod'
import type { PluginPackageHostRequest, PluginPackageHostResponse } from '../../../packages/runtime/src/contracts/plugin-package-host'
import { createCanvasController } from './canvas-controller'

export function registerCanvasIpc(
  trustedSender: (event: Electron.IpcMainInvokeEvent) => boolean,
  packageHost: (request: PluginPackageHostRequest) => Promise<PluginPackageHostResponse>
) {
  const controllers = new WeakMap<Electron.WebContents, { frame: Electron.WebFrameMain; controller: ReturnType<typeof createCanvasController> }>()
  ipcMain.handle('canvas:request', (event, payload: unknown) => {
    if (!trustedSender(event) || !event.senderFrame || event.senderFrame !== event.sender.mainFrame) return { ok: false, code: 'unavailable' }
    const contents = event.sender, frame = event.senderFrame
    let owned = controllers.get(contents)
    if (!owned || owned.frame !== frame) {
      if (owned) void owned.controller.dispose()
      const controller = createCanvasController({ packageHost, current: () => !contents.isDestroyed() && contents.mainFrame === frame && trustedSender(event) })
      owned = { frame, controller }
      controllers.set(contents, owned)
      contents.once('destroyed', () => { void controller.dispose() })
    }
    return owned.controller.request(payload)
  })
  const picker = z.object({ workspace: z.string().min(1).max(4096).refine(p => isAbsolute(p) && !/[\0\r\n]/.test(p)) }).strict()
  ipcMain.handle('canvas:pick-file', async (event, payload: unknown) => {
    const unavailable = { ok: false as const }
    if (!trustedSender(event) || event.senderFrame !== event.sender.mainFrame) return unavailable
    const parsed = picker.safeParse(payload)
    const owner = BrowserWindow.fromWebContents(event.sender), frame = event.senderFrame
    if (!parsed.success || !owner || owner.isDestroyed()) return unavailable
    const current = () => !owner.isDestroyed() && owner.webContents.mainFrame === frame && trustedSender(event)
    try {
      const packages = await packageHost({ action: 'list' })
      if (!current() || !packages.ok || !('packages' in packages) || !packages.packages.some(p => p.packageId === 'analytix-canvas' && p.available && p.desiredState === 'enabled')) return unavailable
      const picked = await dialog.showOpenDialog(owner, { title: '打开画布或图片', defaultPath: parsed.data.workspace,
        filters: [{ name: 'Canvas / PNG', extensions: ['canvas', 'png'] }], properties: ['openFile', 'dontAddToRecent'] })
      if (!current()) return unavailable
      if (picked.canceled) return { ok: true as const, path: null }
      if (picked.filePaths.length !== 1 || !isAbsolute(picked.filePaths[0]) || !/\.(canvas|png)$/.test(extname(picked.filePaths[0]).toLowerCase())) return unavailable
      return { ok: true as const, path: picked.filePaths[0] }
    } catch { return unavailable }
  })
}
