import { clipboard, dialog, ipcMain, type BrowserWindow, type IpcMainInvokeEvent } from 'electron'
import { writeFile } from 'node:fs/promises'
import {
  TOOL_RESULT_LOCAL_DISPLAY_PATH_V1, TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1,
  toolResultLocalDisplaySchemaV1, toolResultLocalViewRequestSchemaV1,
  toolResultLocalEffectRequestSchemaV1,
  type ToolResultLocalDisplayV1, type ToolResultLocalSelectorV1,
  type ToolResultLocalViewResultV1, type ToolResultLocalEffectResultV1
} from '../../../packages/runtime/src/contracts/tool-result-local-display'
import { captureNativeRendererDocument } from './native-renderer-document'
import { projectOrdinaryWriteExportContentV1 } from '../services/write-export-content'

type Owner = { isCurrent: () => boolean; release: () => void; signal: AbortSignal }
type View = { selector: ToolResultLocalSelectorV1; digest?: string; owner: Owner }
type Transport = (path: string, body: string, signal?: AbortSignal) => Promise<{ ok: boolean; status: number; body: string }>
const invalidators = new Set<() => void>()
let hostTransitionDepth = 0
export async function withToolResultLocalDisplayTransitionV1<T>(operation: () => Promise<T>): Promise<T> {
  hostTransitionDepth += 1
  try { invalidateToolResultLocalDisplaysV1(); return await operation() }
  finally {
    try { invalidateToolResultLocalDisplaysV1() }
    finally { hostTransitionDepth -= 1 }
  }
}
export function invalidateToolResultLocalDisplaysV1(): void {
  for (const invalidate of [...invalidators]) {
    try { invalidate() } catch { /* Continue retiring independent document owners. */ }
  }
}
const unavailable = (): { ok: false; code: 'unavailable' } => ({ ok: false, code: 'unavailable' })
// SGR decoration can split an identifier or authority token. Rejoin that
// ordinary text before the existing Write projection. Other terminal control
// languages need their own verified projection and cannot enter this effect.
function ordinaryToolExportTextV1(body: string): string {
  const text = body.replace(/\x1b\[[0-9;]*m/g, '')
  if (/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/u.test(text)) throw new Error('Unsupported terminal control content.')
  return projectOrdinaryWriteExportContentV1(text)
}
const selectorOf = ({ threadId, turnId, callId, resultItemId }: ToolResultLocalSelectorV1): ToolResultLocalSelectorV1 =>
  ({ threadId, turnId, callId, resultItemId })

// Retains only document leases, identities and immutable hashes; no body cache.
export class ToolResultLocalDisplayHostV1 {
  private views = new Map<string, View>()
  constructor(private transport: Transport, private notify: (viewId?: string) => void) {}
  close(viewId: string): void {
    const view = this.views.get(viewId)
    this.views.delete(viewId)
    try { view?.owner.release() } catch { /* The lease is already detached. */ }
  }
  invalidate(): void {
    for (const id of [...this.views.keys()]) this.close(id)
    this.notifySafely()
  }
  private notifySafely(viewId?: string): void {
    try { this.notify(viewId) } catch { /* A closed document cannot acknowledge cleanup. */ }
  }
  private fail(viewId: string, expected: View): { ok: false; code: 'unavailable' } {
    if (this.views.get(viewId) !== expected) return unavailable()
    this.close(viewId)
    this.notifySafely(viewId)
    return unavailable()
  }
  private current(viewId: string, view: View): boolean {
    return this.views.get(viewId) === view && view.owner.isCurrent() && !view.owner.signal.aborted
  }
  private async fresh(viewId: string, view: View): Promise<ToolResultLocalDisplayV1 | null> {
    if (!this.current(viewId, view)) return null
    try {
      const response = await this.transport(TOOL_RESULT_LOCAL_DISPLAY_PATH_V1, JSON.stringify(view.selector), view.owner.signal)
      if (!this.current(viewId, view) || !response.ok || response.status !== 200 ||
          Buffer.byteLength(response.body, 'utf8') > TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1) return null
      const parsed = toolResultLocalDisplaySchemaV1.safeParse(JSON.parse(response.body))
      if (!parsed.success || parsed.data.resultItemId !== view.selector.resultItemId ||
          view.digest !== undefined && parsed.data.snapshotDigest !== view.digest) return null
      return parsed.data
    } catch { return null }
  }
  async open(payload: unknown, owner: Owner): Promise<ToolResultLocalViewResultV1> {
    const request = toolResultLocalViewRequestSchemaV1.safeParse(payload)
    if (!request.success || !owner.isCurrent()) { owner.release(); return unavailable() }
    const { viewId } = request.data
    this.close(viewId)
    if (this.views.size >= 32) { owner.release(); return unavailable() }
    const view: View = { selector: selectorOf(request.data), owner }
    this.views.set(viewId, view)
    const display = await this.fresh(viewId, view)
    if (!display || !this.current(viewId, view)) return this.fail(viewId, view)
    view.digest = display.snapshotDigest
    return { ok: true, display }
  }
  async effect(
    payload: unknown, action: 'copy' | 'save', current: () => boolean,
    pickDestination: () => Promise<string | null>,
    copy: (text: string) => void, save: (path: string, text: string) => Promise<void>
  ): Promise<ToolResultLocalEffectResultV1> {
    const request = toolResultLocalEffectRequestSchemaV1.safeParse(payload)
    if (!request.success || !current()) return unavailable()
    const { viewId, snapshotDigest } = request.data
    const view = this.views.get(viewId)
    if (!view || view.digest !== snapshotDigest ||
        JSON.stringify(view.selector) !== JSON.stringify(selectorOf(request.data))) return unavailable()
    let destination: string | null = null
    if (action === 'save') {
      // Check authority before opening the dialog and again after it returns.
      if (!await this.fresh(viewId, view) || !current()) return this.fail(viewId, view)
      try { destination = await pickDestination() } catch { return unavailable() }
      if (destination === null) return { ok: false, code: 'canceled' }
    }
    const first = await this.fresh(viewId, view)
    if (!first || !current()) return this.fail(viewId, view)
    let projected: string
    try { projected = ordinaryToolExportTextV1(first.capture.body) }
    catch { return { ok: false, code: 'unsupported' } }
    if (first.capture.body !== '' && projected === '') return { ok: false, code: 'unsupported' }
    const final = await this.fresh(viewId, view)
    if (!final || final.capture.body !== first.capture.body || !current() || !this.current(viewId, view)) return this.fail(viewId, view)
    try {
      if (action === 'copy') copy(projected)
      else await save(destination!, projected)
      return { ok: true }
    } catch { return unavailable() }
  }
}

export function registerToolResultLocalDisplayIpcV1(
  getMainWindow: () => BrowserWindow | null, transport: Transport
): void {
  const hosts = new Map<number, ToolResultLocalDisplayHostV1>()
  const observedWindows = new WeakSet<BrowserWindow>()
  const current = (event: IpcMainInvokeEvent): boolean => {
    const window = getMainWindow()
    return Boolean(hostTransitionDepth === 0 && window && !window.isDestroyed() && window.isVisible() && !window.isMinimized() && window.webContents === event.sender &&
      !event.sender.isDestroyed() && event.senderFrame != null && event.senderFrame === event.sender.mainFrame)
  }
  const hostFor = (event: IpcMainInvokeEvent) => {
    const window = getMainWindow()
    if (window && !observedWindows.has(window)) {
      observedWindows.add(window)
      window.on('hide', invalidateToolResultLocalDisplaysV1)
      window.on('minimize', invalidateToolResultLocalDisplaysV1)
      window.once('closed', () => {
        window.removeListener('hide', invalidateToolResultLocalDisplaysV1)
        window.removeListener('minimize', invalidateToolResultLocalDisplaysV1)
      })
    }
    let host = hosts.get(event.sender.id)
    if (!host) {
      host = new ToolResultLocalDisplayHostV1(transport, viewId => {
        if (!event.sender.isDestroyed()) event.sender.send('tool-result-local:invalidated', viewId ?? null)
      })
      hosts.set(event.sender.id, host)
      const invalidate = () => host!.invalidate()
      invalidators.add(invalidate)
      event.sender.once('destroyed', () => { invalidate(); invalidators.delete(invalidate); hosts.delete(event.sender.id) })
    }
    return host
  }
  ipcMain.handle('tool-result-local:open', async (event, payload: unknown) => {
    if (!current(event)) return unavailable()
    const host = hostFor(event)
    const controller = new AbortController()
    const lease = captureNativeRendererDocument(event.sender, () => current(event), () => { controller.abort(); host.invalidate() })
    return host.open(payload, {
      signal: controller.signal,
      isCurrent: lease.isCurrent,
      release: () => { controller.abort(); lease.release() }
    })
  })
  ipcMain.handle('tool-result-local:invalidate', event => {
    if (!current(event)) throw new Error('Tool display authority unavailable.')
    invalidateToolResultLocalDisplaysV1()
  })
  ipcMain.handle('tool-result-local:close', (event, viewId: unknown) => {
    if (current(event) && typeof viewId === 'string') hosts.get(event.sender.id)?.close(viewId)
  })
  for (const action of ['copy', 'save'] as const) {
    ipcMain.handle('tool-result-local:' + action, async (event, payload: unknown) => {
      if (!current(event)) return unavailable()
      return hostFor(event).effect(payload, action, () => current(event), async () => {
        const window = getMainWindow()
        if (!window || !current(event)) return null
        const result = await dialog.showSaveDialog(window, {
          defaultPath: 'tool-result-redacted.txt', filters: [{ name: 'Text', extensions: ['txt'] }]
        })
        return result.canceled || !result.filePath ? null : result.filePath
      }, text => clipboard.writeText(text), (path, text) => writeFile(path, text, { encoding: 'utf8', mode: 0o600 }))
    })
  }
}
