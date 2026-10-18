import { ipcMain, type BrowserWindow, type IpcMainInvokeEvent } from 'electron'
import { userInputLocalSelectorSchemaV1 } from '../../../packages/runtime/src/contracts/user-input-local-display'
import { captureNativeRendererDocument } from './native-renderer-document'
import { UserInputWaitOwnerV1, type UserInputWaitTransportV1 } from '../runtime/user-input-wait-owner'

const invalidators = new Set<() => void>()
let transitionDepth = 0
export function beginUserInputWaitTransitionV1(): () => void {
  transitionDepth++
  for (const invalidate of [...invalidators]) invalidate()
  let active = true
  return () => { if (active) { active = false; transitionDepth-- } }
}
export function registerUserInputWaitIpcV1(getMainWindow: () => BrowserWindow | null, transport?: UserInputWaitTransportV1): void {
  const hosts = new Map<number, UserInputWaitOwnerV1>()
  const current = (event: IpcMainInvokeEvent): boolean => {
    const window = getMainWindow()
    return Boolean(window && !window.isDestroyed() && window.webContents === event.sender &&
      !event.sender.isDestroyed() && event.senderFrame != null && event.senderFrame === event.sender.mainFrame)
  }
  ipcMain.handle('user-input-wait:open', (event, payload: unknown) => {
    if (!transport || transitionDepth !== 0 || !current(event)) return { state: 'unavailable' as const }
    let host = hosts.get(event.sender.id)
    if (!host) {
      host = new UserInputWaitOwnerV1(transport, changed => {
        if (!event.sender.isDestroyed()) event.sender.send('user-input-wait:changed', changed)
      })
      hosts.set(event.sender.id, host)
      const invalidate = () => host!.invalidate()
      invalidators.add(invalidate)
      event.sender.once('destroyed', () => { invalidate(); invalidators.delete(invalidate); hosts.delete(event.sender.id) })
    }
    const selected = host
    const lease = captureNativeRendererDocument(event.sender, () => current(event), () => selected.invalidate())
    return host.open(payload, lease)
  })
  ipcMain.handle('user-input-wait:close', (event, payload: unknown) => {
    const parsed = userInputLocalSelectorSchemaV1.safeParse(payload)
    if (current(event) && parsed.success) hosts.get(event.sender.id)?.close(parsed.data)
  })
}
