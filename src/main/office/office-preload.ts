import { contextBridge, ipcRenderer } from 'electron'
import { OFFICE_BIND_IPC, isOfficeEvent, isOfficeEngineFailure, isOfficeReady, isOfficeRequest, isOfficeResult, officeRecord, officeToken, type OfficeBinding, type OfficeEngineFailure, type OfficeEvent, type OfficeReady, type OfficeRequest, type OfficeResult } from './office-protocol'

type SurfaceMessage = OfficeEngineFailure | OfficeReady | OfficeResult | OfficeEvent
type SurfaceListener = (message: OfficeBinding | OfficeRequest) => void
let bound = false, listener: SurfaceListener | undefined, port: MessagePort | undefined, channel: string | undefined
let connected = false
function deliverBinding(): void {
  if (port && listener && channel && !connected) { connected = true; listener({ type: 'bind', channel }) }
}
ipcRenderer.on(OFFICE_BIND_IPC, (event, value: unknown) => {
  if (bound || !officeRecord(value) || Object.keys(value).sort().join(',') !== 'channel,entryURL' || !officeToken(value.channel) || value.entryURL !== location.href || location.protocol !== 'http:' || location.hostname !== '127.0.0.1' || event.ports.length !== 1) return
  bound = true; channel = value.channel; port = event.ports[0]
  port.onmessage = ({ data }) => {
    if (!isOfficeRequest(data) || data.channel !== channel || !connected) return
    listener?.(data)
  }
  port.start()
  deliverBinding()
})
// The transferred port stays inside the isolated preload. This facade has no
// IPC channel/path/URL argument and cannot invoke arbitrary Main operations.
contextBridge.exposeInMainWorld('analytixOfficeSurface', Object.freeze({
  connect(handler: SurfaceListener): void {
    if (listener || typeof handler !== 'function') throw Error('office-transport-unavailable')
    listener = handler; deliverBinding()
  },
  send(message: SurfaceMessage): void {
    if (!port || !connected || message?.channel !== channel || !(isOfficeEngineFailure(message) || isOfficeReady(message) || isOfficeResult(message) || isOfficeEvent(message))) throw Error('office-invalid-message')
    port.postMessage(message)
  }
}))
addEventListener('unload', () => { port?.close(); port = undefined; listener = undefined; channel = undefined })
