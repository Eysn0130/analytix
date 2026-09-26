import { BrowserWindow, dialog, ipcMain } from 'electron'
import { extname, isAbsolute } from 'node:path'
import { z } from 'zod'
import type { PluginPackageHostRequest, PluginPackageHostResponse } from '../../../packages/runtime/src/contracts/plugin-package-host'
import { createCanvasController } from './canvas-controller'
import { captureNativeRendererDocument } from '../ipc/native-renderer-document'

export function registerCanvasIpc(
  trustedSender: (event: Electron.IpcMainInvokeEvent) => boolean,
  packageHost: (request: PluginPackageHostRequest) => Promise<PluginPackageHostResponse>
) {
  type OwnedController = { lease: ReturnType<typeof captureNativeRendererDocument>; controller: ReturnType<typeof createCanvasController>; retiring?: Promise<void> }
  const controllers = new WeakMap<Electron.WebContents, OwnedController>()
  const retire = (owned: OwnedController) => owned.retiring ??= owned.controller.dispose()
  ipcMain.handle('canvas:request', async (event, payload: unknown) => {
    if (!trustedSender(event) || !event.senderFrame || event.senderFrame !== event.sender.mainFrame) return { ok: false, code: 'unavailable' }
    const contents = event.sender, frame = event.senderFrame
    const current = () => !contents.isDestroyed() && contents.mainFrame === frame && trustedSender(event)
    const requestOwner = captureNativeRendererDocument(contents, current)
    try {
      let owned = controllers.get(contents)
      while (owned && !owned.lease.isCurrent()) {
        owned.lease.release()
        // Core may reuse an object session ID. Finish the old owner's close
        // before a replacement document can open that same object again.
        await retire(owned)
        if (!requestOwner.isCurrent()) return { ok: false, code: 'unavailable' }
        if (controllers.get(contents) === owned) controllers.delete(contents)
        owned = controllers.get(contents)
      }
      if (!owned) {
        const lease = captureNativeRendererDocument(contents, current, () => { void retire(created) })
        const controller = createCanvasController({ packageHost, current: lease.isCurrent })
        const created: OwnedController = { lease, controller }
        owned = created
        controllers.set(contents, owned)
      }
      const result = await owned.controller.request(payload)
      return requestOwner.isCurrent() ? result : { ok: false, code: 'unavailable' }
    } finally { requestOwner.release() }
  })
  const picker = z.object({ workspace: z.string().min(1).max(4096).refine(p => isAbsolute(p) && !/[\0\r\n]/.test(p)) }).strict()
  ipcMain.handle('canvas:pick-file', async (event, payload: unknown) => {
    const unavailable = { ok: false as const }
    if (!trustedSender(event) || event.senderFrame !== event.sender.mainFrame) return unavailable
    const parsed = picker.safeParse(payload)
    const owner = BrowserWindow.fromWebContents(event.sender), frame = event.senderFrame
    if (!parsed.success || !owner || owner.isDestroyed()) return unavailable
    const lease = captureNativeRendererDocument(event.sender, () => !owner.isDestroyed() && owner.webContents.mainFrame === frame && trustedSender(event))
    const current = lease.isCurrent
    try {
      const packages = await packageHost({ action: 'list' })
      if (!current() || !packages.ok || !('packages' in packages) || !packages.packages.some(p => p.packageId === 'analytix-canvas' && p.available && p.desiredState === 'enabled')) return unavailable
      const picked = await dialog.showOpenDialog(owner, { title: '打开画布或图片', defaultPath: parsed.data.workspace,
        filters: [{ name: 'Canvas / PNG', extensions: ['canvas', 'png'] }], properties: ['openFile', 'dontAddToRecent'] })
      if (!current()) return unavailable
      if (picked.canceled) return { ok: true as const, path: null }
      if (picked.filePaths.length !== 1 || !isAbsolute(picked.filePaths[0]) || !/\.(canvas|png)$/.test(extname(picked.filePaths[0]).toLowerCase())) return unavailable
      return { ok: true as const, path: picked.filePaths[0] }
    } catch { return unavailable } finally { lease.release() }
  })
}
