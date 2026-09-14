import { randomUUID } from 'node:crypto'
import type { BrowserWindow, Event, IpcMain, WebFrameMain } from 'electron'
import { WRITE_SHUTDOWN_ACK, WRITE_SHUTDOWN_REQUEST, writeShutdownAckSchema } from '../shared/write-shutdown'

/** One owner for both window destruction and application shutdown. Core must
 * remain running until prepareQuit succeeds. Only app renderer windows enroll. */
export function createWriteShutdownCoordinator(options: {
  ipc: Pick<IpcMain, 'handle'>
  prepareNative: (window?: BrowserWindow) => Promise<boolean>
  cancelNative: (window?: BrowserWindow) => Promise<void>
  notifyBlocked: (window: BrowserWindow) => Promise<void>
  timeoutMs?: number
}) {
  const windows = new Set<BrowserWindow>()
  const prepared = new Map<BrowserWindow, { requestId: string; frame: WebFrameMain }>()
  const cancellations = new Map<BrowserWindow, { requestId: string; frame: WebFrameMain }>()
  const pending = new Map<string, { window: BrowserWindow; frame: WebFrameMain; phase: 'prepare' | 'cancel'; finish: (ok: boolean) => void }>()
  let operation: Promise<boolean> | undefined
  let quitRequested = false
  let quitReady = false
  let replay: BrowserWindow | undefined
  let replayEvent: Event | undefined
  const canCreateWindow = () => !quitRequested && !quitReady
  const canNavigateWindow = (window: BrowserWindow) => canCreateWindow() && !prepared.has(window) && !cancellations.has(window)
  const preparedFrameIsCurrent = (window: BrowserWindow) => {
    const attempt = prepared.get(window)
    return !!attempt && current(window, attempt.frame) && !window.webContents.isLoadingMainFrame()
  }
  const current = (window: BrowserWindow, frame: WebFrameMain) => windows.has(window) && !window.isDestroyed() && window.webContents.mainFrame === frame
  options.ipc.handle(WRITE_SHUTDOWN_ACK, (event, value: unknown) => {
    const parsed = writeShutdownAckSchema.safeParse(value)
    if (!parsed.success) return { ok: false }
    const attempt = pending.get(parsed.data.requestId)
    if (!attempt || parsed.data.phase !== attempt.phase || !current(attempt.window, attempt.frame) ||
      event.sender !== attempt.window.webContents || event.senderFrame !== attempt.frame) return { ok: false }
    attempt.finish(parsed.data.outcome.result === 'ready')
    return { ok: true }
  })
  const handshake = (window: BrowserWindow, requestId: string, phase: 'prepare' | 'cancel', frame: WebFrameMain) => new Promise<boolean>(resolve => {
    pending.get(requestId)?.finish(false)
    const finish = (ok: boolean) => { clearTimeout(timer); pending.delete(requestId); resolve(ok) }
    const timer = setTimeout(() => finish(false), options.timeoutMs ?? 15_000)
    pending.set(requestId, { window, frame, phase, finish })
    try {
      if (!current(window, frame)) { finish(false); return }
      window.webContents.send(WRITE_SHUTDOWN_REQUEST, { requestId, phase })
    } catch { finish(false) }
  })
  const cancel = async () => {
    quitReady = false
    // Cancel even an unacknowledged prepare: Renderer may already be frozen.
    for (const [window, attempt] of prepared) cancellations.set(window, attempt)
    const attempts = [...cancellations.entries()]
    prepared.clear()
    await Promise.allSettled([
      options.cancelNative(),
      ...attempts.map(async ([window, attempt]) => {
        if (await handshake(window, attempt.requestId, 'cancel', attempt.frame) && cancellations.get(window) === attempt) cancellations.delete(window)
      })
    ])
    quitRequested = false
  }
  const prepare = async (targets: BrowserWindow[], quitting: boolean): Promise<boolean> => {
    let accepted = false
    let textReady = false
    try {
      for (const window of targets) {
        if (window.isDestroyed() || window.webContents.isLoadingMainFrame()) return false
        const abandoned = cancellations.get(window)
        if (abandoned) {
          // An unacknowledged cancellation may have left the old Renderer hold
          // alive. Resolve it before issuing a new prepare token.
          if (window.webContents.mainFrame === abandoned.frame && !await handshake(window, abandoned.requestId, 'cancel', abandoned.frame)) return false
          cancellations.delete(window)
        }
        const previous = prepared.get(window)
        if (previous) {
          if (!current(window, previous.frame)) return false
          continue
        }
        const attempt = { requestId: randomUUID(), frame: window.webContents.mainFrame }
        prepared.set(window, attempt)
        if (!await handshake(window, attempt.requestId, 'prepare', attempt.frame)) return false
      }
      textReady = true
      if (!await options.prepareNative(quitting ? undefined : targets[0])) return false
      accepted = targets.every(window => {
        const attempt = prepared.get(window)
        return !!attempt && current(window, attempt.frame) && !window.webContents.isLoadingMainFrame()
      }) && (!quitting || windows.size === targets.length)
      return accepted
    } catch { return false } finally {
      if (!accepted) {
        await cancel()
        const window = targets.find(window => !window.isDestroyed())
        if (window && !textReady) await options.notifyBlocked(window).catch(() => undefined)
      }
    }
  }
  const trackWindow = (window: BrowserWindow) => {
    windows.add(window)
    window.webContents.on('will-navigate', event => {
      if (!canNavigateWindow(window)) event.preventDefault()
    })
    window.webContents.on('will-frame-navigate', event => {
      if (event.isMainFrame && !canNavigateWindow(window)) event.preventDefault()
    })
    window.webContents.on('will-redirect', event => {
      if (event.isMainFrame && !canNavigateWindow(window)) event.preventDefault()
    })
    // Renderer window.open bypasses the application's createWindow function.
    window.webContents.setWindowOpenHandler(() => ({ action: canCreateWindow() ? 'allow' : 'deny' }))
    window.on('close', event => {
      if (replay === window) replayEvent = event
      // The earlier tray/ask policy retains ownership of non-destructive close.
      if (event.defaultPrevented) return
      if ((replay === window || quitReady) && preparedFrameIsCurrent(window)) return
      event.preventDefault()
      if (operation || quitRequested) return
      const closing = prepare([window], false)
      operation = closing
      void closing.then(async accepted => {
        if (accepted && !quitRequested && !window.isDestroyed()) {
          replay = window
          replayEvent = undefined
          try { window.close() } finally { replay = undefined }
          const prevented = () => replayEvent?.defaultPrevented
          if (prevented()) await cancel()
        }
      }).catch(() => cancel()).finally(() => { if (operation === closing) operation = undefined })
    })
    window.once('closed', () => {
      windows.delete(window)
      prepared.delete(window)
      cancellations.delete(window)
      for (const attempt of pending.values()) if (attempt.window === window) attempt.finish(false)
    })
  }
  const prepareQuit = async (): Promise<boolean> => {
    quitRequested = true
    if (operation && !await operation) return false
    // A preceding window-close prepare may have cancelled while we awaited it.
    quitRequested = true
    operation = prepare([...windows], true)
    try { quitReady = await operation; return quitReady } finally { operation = undefined }
  }
  return { trackWindow, prepareQuit, cancel, canCreateWindow, canNavigateWindow }
}
